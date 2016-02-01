package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
)

type process struct {
	id           string
	bundle       string
	stdio        *stdio
	s            specs.Process
	exec         bool
	containerPid int
	checkpoint   *runtime.Checkpoint
}

func newProcess(id, bundle string, exec bool, checkpoint string) (*process, error) {
	p := &process{
		id:     id,
		bundle: bundle,
		exec:   exec,
	}
	s, err := loadProcess()
	if err != nil {
		return nil, err
	}
	p.s = *s
	if checkpoint != "" {
		cpt, err := loadCheckpoint(bundle, checkpoint)
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

func loadProcess() (*specs.Process, error) {
	f, err := os.Open("process.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s specs.Process
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
	args := []string{
		"--id", p.id,
	}
	if p.exec {
		args = append(args, "exec",
			"--process", filepath.Join(cwd, "process.json"),
			"--console", p.stdio.console,
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
			"--console", p.stdio.console,
		)
	}
	args = append(args,
		"-d",
		"--pid-file", filepath.Join(cwd, "pid"),
	)
	cmd := exec.Command("runc", args...)
	cmd.Dir = p.bundle
	cmd.Stdin = p.stdio.stdin
	cmd.Stdout = p.stdio.stdout
	cmd.Stderr = p.stdio.stderr
	// set the parent death signal to SIGKILL so that if the shim dies the container
	// process also dies
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	if err := cmd.Run(); err != nil {
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
	if !p.exec {
		return exec.Command("runc", "--id", p.id, "delete").Run()
	}
	return nil
}

// openIO opens the pre-created fifo's for use with the container
// in RDWR so that they remain open if the other side stops listening
func (p *process) openIO() error {
	p.stdio = &stdio{}
	if p.s.Terminal {
		// FIXME: this is wrong for user namespaces and will need to be translated
		console, err := libcontainer.NewConsole(int(p.s.User.UID), int(p.s.User.GID))
		if err != nil {
			return err
		}
		p.stdio.console = console.Path()
		stdin, err := os.OpenFile("stdin", syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		go io.Copy(console, stdin)
		stdout, err := os.OpenFile("stdout", syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		go func() {
			io.Copy(stdout, console)
			console.Close()
		}()
		return nil
	}
	i, err := p.initializeIO(int(p.s.User.UID))
	if err != nil {
		return err
	}
	// non-tty
	for name, dest := range map[string]func(f *os.File){
		"stdin": func(f *os.File) {
			go io.Copy(i.Stdin, f)
		},
		"stdout": func(f *os.File) {
			go io.Copy(f, i.Stdout)
		},
		"stderr": func(f *os.File) {
			go io.Copy(f, i.Stderr)
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
	stdin   *os.File
	stdout  *os.File
	stderr  *os.File
	console string
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
