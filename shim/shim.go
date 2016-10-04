package shim

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/docker/containerd/oci"
	"github.com/docker/containerkit"
)

/*
├── libcontainerd
│   ├── containerd
│   │   └── ff2e86955c2be43f0e3c300fbd3786599301bd8efcaa5a386587f132e73af242
│   │       ├── init
│   │       │   ├── control
│   │       │   ├── exit
│   │       │   ├── log.json
│   │       │   ├── pid
│   │       │   ├── process.json
│   │       │   ├── shim-log.json
│   │       │   └── starttime
│   │       └── state.json
│   └── ff2e86955c2be43f0e3c300fbd3786599301bd8efcaa5a386587f132e73af242
│       ├── config.json
│       ├── init-stderr
│       ├── init-stdin
│       └── init-stdout
*/

type Opts struct {
	Name           string
	RuntimeName    string
	RuntimeLogFile string
	RuntimeArgs    []string
	Root           string
	Timeout        time.Duration
}

type state struct {
	Bundle      string   `json:"bundle"`
	Stdin       string   `json:"stdin"`
	Stdout      string   `json:"stdout"`
	Stderr      string   `json:"stderr"`
	Runtime     string   `json:"runtime"`
	RuntimeArgs []string `json:"runtimeArgs"`
	Shim        string   `json:"shim"`
	NoPivotRoot bool     `json:"noPivotRoot"`
}

func New(opts Opts) (*Shim, error) {
	if err := os.MkdirAll(opts.Root, 0711); err != nil {
		return nil, err
	}
	r, err := oci.New(oci.Opts{
		Name:    opts.RuntimeName,
		LogFile: opts.RuntimeLogFile,
		Args:    opts.RuntimeArgs,
	})
	if err != nil {
		return nil, err
	}
	return &Shim{
		root:    opts.Root,
		name:    opts.Name,
		timeout: opts.Timeout,
		runtime: r,
	}, nil
}

// Load will load an existing shim with all its information restored from the
// provided path
func Load(path string) (*Shim, error) {

}

// Shim is a container runtime that adds a shim process as the container's parent
// to hold open stdio and other resources so that higher level daemons can exit and
// load running containers for handling upgrades and/or crashes
//
// The shim uses an OCI compliant runtime as its executor
type Shim struct {
	// root holds runtime state information for the containers
	// launched by the runtime
	root string
	// name is the name of the runtime, i.e. runc
	name    string
	timeout time.Duration

	runtime       *oci.OCIRuntime
	pmu           sync.Mutex
	initProcesses map[string]*process
}

func (s *Shim) Create(c *containerkit.Container) (containerkit.ProcessDelegate, error) {
	if err := os.Mkdir(filepath.Join(c.root, c.id), 0711); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(c.root, c.id, StateFile))
	if err != nil {
		return nil, err
	}
	err = json.NewEncoder(f).Encode(state{
		Bundle:      c.bundle,
		Labels:      c.labels,
		Runtime:     c.runtime,
		RuntimeArgs: c.runtimeArgs,
		Shim:        c.shim,
		NoPivotRoot: opts.NoPivotRoot,
	})
	f.Close()
	if err != nil {
		return nil, err
	}
	cmd := s.command(c.ID(), c.Path(), s.runtime.Name())
	// exec the shim inside the state directory setup with the process
	// information for what is being run
	cmd.Dir = processRoot
	// make sure the shim is in a new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	p, err := s.startCommand("init", cmd)
	if err != nil {
		return nil, err
	}
	s.pmu.Lock()
	s.initProcesses[c.ID()] = p
	s.pmu.Unlock()
	// ~TODO: oom and stats stuff here
	return p, nil
}

func (s *Shim) Start(c *containerkit.Container) error {
	p, err := s.getContainerInit(c)
	if err != nil {
		return err
	}
	var (
		errC = make(chan error, 1)
		cmd  = s.runtime.Command("start", c.ID())
	)
	go func() {
		out, err := cmd.CombinedOutput()
		if err != nil {
			errC <- fmt.Errorf("%s: %q", err, out)
		}
		errC <- nil
	}()
	select {
	case err := <-errC:
		if err != nil {
			return err
		}
	case <-p.done:
		if !p.success {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			cmd.Wait()
			return ErrShimExited
		}
		err := <-errC
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Shim) getContainerInit(c *containerkit.Container) (*process, error) {
	s.pmu.Lock()
	p, ok := s.initProcesses[c.ID()]
	s.pmu.Unlock()
	if !ok {
		return nil, errInitProcessNotExist
	}
	return p, nil
}

func (s *Shim) startCommand(processName string, cmd *exec.Cmd) (*process, error) {
	p := &process{
		name:    processName,
		cmd:     cmd,
		done:    make(chan struct{}),
		timeout: s.timeout,
	}
	if err := cmd.Start(); err != nil {
		close(proc.done)
		if checkShimNotFound(err) {
			return fmt.Errorf("%s not install on system", s.name)
		}
		return nil, err
	}
	// make sure it does not die before we get the container's pid
	defer func() {
		go p.checkExited()
	}()
	if err := p.waitForCreate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Shim) command(args ...string) *exec.Cmd {
	return exec.Command(s.name, args...)
}

// checkShimNotFound checks the error returned from a exec call to see if the binary
// that was called exists on the system and returns true if the shim binary does not exist
func checkShimNotFound(err error) bool {
	if exitError, ok := err.(*exec.Error); ok {
		e := exitError.Err
		return e == exec.ErrNotFound || e == os.ErrNotExist
	}
	return false
}
