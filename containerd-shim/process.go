package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/runc/libcontainer"
)

type process struct {
	sync.WaitGroup
	id           string
	bundle       string
	stdio        *stdio
	exec         bool
	containerPid int
	checkpoint   *runtime.Checkpoint
	shimIO       *IO
	console      libcontainer.Console
	consolePath  string
	state        *runtime.ProcessState
	runtime      string
}

func newProcess(id, bundle, runtimeName string) (*process, error) {
	p := &process{
		id:      id,
		bundle:  bundle,
		runtime: runtimeName,
	}
	s, err := loadProcess()
	if err != nil {
		return nil, err
	}
	p.state = s
	if s.Checkpoint != "" {
		cpt, err := loadCheckpoint(bundle, s.Checkpoint)
		if err != nil {
			return nil, err
		}
		p.checkpoint = cpt
	}
	if err := p.openIO(); err != nil {
		return nil, err
	}
	return p, nil
}

func loadProcess() (*runtime.ProcessState, error) {
	f, err := os.Open("process.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s runtime.ProcessState
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func loadCheckpoint(bundle, name string) (*runtime.Checkpoint, error) {
	f, err := os.Open(filepath.Join(bundle, "checkpoints", name, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cpt runtime.Checkpoint
	if err := json.NewDecoder(f).Decode(&cpt); err != nil {
		return nil, err
	}
	return &cpt, nil
}

func (p *process) start() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	logPath := filepath.Join(cwd, "log.json")
	args := []string{
		"--log", logPath,
		"--log-format", "json",
	}
	if p.state.Exec {
		args = append(args, "exec",
			"--process", filepath.Join(cwd, "process.json"),
			"--console", p.consolePath,
		)
	} else if p.checkpoint != nil {
		args = append(args, "restore",
			"--image-path", filepath.Join(p.bundle, "checkpoints", p.checkpoint.Name),
		)
		add := func(flags ...string) {
			args = append(args, flags...)
		}
		if p.checkpoint.Shell {
			add("--shell-job")
		}
		if p.checkpoint.Tcp {
			add("--tcp-established")
		}
		if p.checkpoint.UnixSockets {
			add("--ext-unix-sk")
		}
	} else {
		args = append(args, "start",
			"--bundle", p.bundle,
			"--console", p.consolePath,
		)
	}
	args = append(args,
		"-d",
		"--pid-file", filepath.Join(cwd, "pid"),
		p.id,
	)
	cmd := exec.Command(p.runtime, args...)
	cmd.Dir = p.bundle
	cmd.Stdin = p.stdio.stdin
	cmd.Stdout = p.stdio.stdout
	cmd.Stderr = p.stdio.stderr
	// set the parent death signal to SIGKILL so that if the shim dies the container
	// process also dies
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.stdio.stdout.Close()
	p.stdio.stderr.Close()
	if err := cmd.Wait(); err != nil {
		return err
	}
	data, err := ioutil.ReadFile("pid")
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}
	p.containerPid = pid
	return nil
}

func (p *process) pid() int {
	return p.containerPid
}

func (p *process) delete() error {
	if !p.state.Exec {
		out, err := exec.Command(p.runtime, "delete", p.id).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %v", out, err)
		}
	}
	return nil
}

// openIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func (p *process) openIO() error {
	p.stdio = &stdio{}
	var (
		uid = p.state.RootUID
		gid = p.state.RootGID
	)
	if p.state.Terminal {
		console, err := libcontainer.NewConsole(uid, gid)
		if err != nil {
			return err
		}
		p.console = console
		p.consolePath = console.Path()
		stdin, err := os.OpenFile(p.state.Stdin, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		go io.Copy(console, stdin)
		stdout, err := os.OpenFile(p.state.Stdout, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		p.Add(1)
		go func() {
			io.Copy(stdout, console)
			console.Close()
			p.Done()
		}()
		return nil
	}
	i, err := p.initializeIO(uid)
	if err != nil {
		return err
	}
	p.shimIO = i
	// non-tty
	for name, dest := range map[string]func(f *os.File){
		p.state.Stdin: func(f *os.File) {
			go io.Copy(i.Stdin, f)
		},
		p.state.Stdout: func(f *os.File) {
			p.Add(1)
			go func() {
				io.Copy(f, i.Stdout)
				p.Done()
			}()
		},
		p.state.Stderr: func(f *os.File) {
			p.Add(1)
			go func() {
				io.Copy(f, i.Stderr)
				p.Done()
			}()
		},
	} {
		f, err := os.OpenFile(name, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		dest(f)
	}
	return nil
}

type IO struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

func (p *process) initializeIO(rootuid int) (i *IO, err error) {
	var fds []uintptr
	i = &IO{}
	// cleanup in case of an error
	defer func() {
		if err != nil {
			for _, fd := range fds {
				syscall.Close(int(fd))
			}
		}
	}()
	// STDIN
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	fds = append(fds, r.Fd(), w.Fd())
	p.stdio.stdin, i.Stdin = r, w
	// STDOUT
	if r, w, err = os.Pipe(); err != nil {
		return nil, err
	}
	fds = append(fds, r.Fd(), w.Fd())
	p.stdio.stdout, i.Stdout = w, r
	// STDERR
	if r, w, err = os.Pipe(); err != nil {
		return nil, err
	}
	fds = append(fds, r.Fd(), w.Fd())
	p.stdio.stderr, i.Stderr = w, r
	// change ownership of the pipes incase we are in a user namespace
	for _, fd := range fds {
		if err := syscall.Fchown(int(fd), rootuid, rootuid); err != nil {
			return nil, err
		}
	}
	return i, nil
}
func (p *process) Close() error {
	return p.stdio.Close()
}

type stdio struct {
	stdin  *os.File
	stdout *os.File
	stderr *os.File
}

func (s *stdio) Close() error {
	err := s.stdin.Close()
	if oerr := s.stdout.Close(); err == nil {
		err = oerr
	}
	if oerr := s.stderr.Close(); err == nil {
		err = oerr
	}
	return err
}
