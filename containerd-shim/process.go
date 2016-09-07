package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/docker/containerd/specs"
)

var errRuntime = errors.New("shim: runtime execution error")

type checkpoint struct {
	// Timestamp is the time that checkpoint happened
	Created time.Time `json:"created"`
	// Name is the name of the checkpoint
	Name string `json:"name"`
	// TCP checkpoints open tcp connections
	TCP bool `json:"tcp"`
	// UnixSockets persists unix sockets in the checkpoint
	UnixSockets bool `json:"unixSockets"`
	// Shell persists tty sessions in the checkpoint
	Shell bool `json:"shell"`
	// Exit exits the container after the checkpoint is finished
	Exit bool `json:"exit"`
	// EmptyNS tells CRIU not to restore a particular namespace
	EmptyNS []string `json:"emptyNS,omitempty"`
}

type processState struct {
	specs.ProcessSpec
	Exec           bool     `json:"exec"`
	Stdin          string   `json:"containerdStdin"`
	Stdout         string   `json:"containerdStdout"`
	Stderr         string   `json:"containerdStderr"`
	RuntimeArgs    []string `json:"runtimeArgs"`
	NoPivotRoot    bool     `json:"noPivotRoot"`
	CheckpointPath string   `json:"checkpoint"`
	RootUID        int      `json:"rootUID"`
	RootGID        int      `json:"rootGID"`
}

type process struct {
	sync.WaitGroup
	id             string
	bundle         string
	stdio          *stdio
	exec           bool
	containerPid   int
	checkpoint     *checkpoint
	checkpointPath string
	shimIO         *IO
	stdinCloser    io.Closer
	console        *os.File
	consolePath    string
	state          *processState
	runtime        string
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
	if s.CheckpointPath != "" {
		cpt, err := loadCheckpoint(s.CheckpointPath)
		if err != nil {
			return nil, err
		}
		p.checkpoint = cpt
		p.checkpointPath = s.CheckpointPath
	}
	if err := p.openIO(); err != nil {
		return nil, err
	}
	return p, nil
}

func loadProcess() (*processState, error) {
	f, err := os.Open("process.json")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s processState
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func loadCheckpoint(checkpointPath string) (*checkpoint, error) {
	f, err := os.Open(filepath.Join(checkpointPath, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cpt checkpoint
	if err := json.NewDecoder(f).Decode(&cpt); err != nil {
		return nil, err
	}
	return &cpt, nil
}

func (p *process) create() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	logPath := filepath.Join(cwd, "log.json")
	args := append([]string{
		"--log", logPath,
		"--log-format", "json",
	}, p.state.RuntimeArgs...)
	if p.state.Exec {
		args = append(args, "exec",
			"-d",
			"--process", filepath.Join(cwd, "process.json"),
			"--console", p.consolePath,
		)
	} else if p.checkpoint != nil {
		args = append(args, "restore",
			"-d",
			"--image-path", p.checkpointPath,
			"--work-path", filepath.Join(p.checkpointPath, "criu.work", "restore-"+time.Now().Format(time.RFC3339)),
		)
		add := func(flags ...string) {
			args = append(args, flags...)
		}
		if p.checkpoint.Shell {
			add("--shell-job")
		}
		if p.checkpoint.TCP {
			add("--tcp-established")
		}
		if p.checkpoint.UnixSockets {
			add("--ext-unix-sk")
		}
		if p.state.NoPivotRoot {
			add("--no-pivot")
		}
		for _, ns := range p.checkpoint.EmptyNS {
			add("--empty-ns", ns)
		}

	} else {
		args = append(args, "create",
			"--bundle", p.bundle,
			"--console", p.consolePath,
		)
		if p.state.NoPivotRoot {
			args = append(args, "--no-pivot")
		}
	}
	args = append(args,
		"--pid-file", filepath.Join(cwd, "pid"),
		p.id,
	)
	cmd := exec.Command(p.runtime, args...)
	cmd.Dir = p.bundle
	cmd.Stdin = p.stdio.stdin
	cmd.Stdout = p.stdio.stdout
	cmd.Stderr = p.stdio.stderr
	// Call out to setPDeathSig to set SysProcAttr as elements are platform specific
	cmd.SysProcAttr = setPDeathSig()

	if err := cmd.Start(); err != nil {
		if exErr, ok := err.(*exec.Error); ok {
			if exErr.Err == exec.ErrNotFound || exErr.Err == os.ErrNotExist {
				return fmt.Errorf("%s not installed on system", p.runtime)
			}
		}
		return err
	}
	p.stdio.stdout.Close()
	p.stdio.stderr.Close()
	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return errRuntime
		}
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
		cmd := exec.Command(p.runtime, append(p.state.RuntimeArgs, "delete", p.id)...)
		cmd.SysProcAttr = setPDeathSig()
		out, err := cmd.CombinedOutput()
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
	go func() {
		if stdinCloser, err := os.OpenFile(p.state.Stdin, syscall.O_WRONLY, 0); err == nil {
			p.stdinCloser = stdinCloser
		}
	}()

	if p.state.Terminal {
		master, console, err := newConsole(uid, gid)
		if err != nil {
			return err
		}
		p.console = master
		p.consolePath = console
		stdin, err := os.OpenFile(p.state.Stdin, syscall.O_RDONLY, 0)
		if err != nil {
			return err
		}
		go io.Copy(master, stdin)
		stdout, err := os.OpenFile(p.state.Stdout, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
		p.Add(1)
		go func() {
			io.Copy(stdout, master)
			master.Close()
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

	f, err := os.OpenFile(p.state.Stdin, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	go func() {
		io.Copy(i.Stdin, f)
		i.Stdin.Close()
	}()

	return nil
}

// IO holds all 3 standard io Reader/Writer (stdin,stdout,stderr)
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
	// change ownership of the pipes in case we are in a user namespace
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
