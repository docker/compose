// +build runc

package runc

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/docker/containerd/runtime"
	"github.com/opencontainers/specs"
)

func NewRuntime(stateDir string) (runtime.Runtime, error) {
	return &runcRuntime{
		stateDir: stateDir,
	}, nil
}

type runcContainer struct {
	id          string
	path        string
	stateDir    string
	exitStatus  int
	processes   map[int]*runcProcess
	initProcess *runcProcess
}

func (c *runcContainer) ID() string {
	return c.id
}

func (c *runcContainer) Start() error {
	return c.initProcess.cmd.Start()
}

func (c *runcContainer) Path() string {
	return c.path
}

func (c *runcContainer) Pid() (int, error) {
	return c.initProcess.cmd.Process.Pid, nil
}

func (c *runcContainer) SetExited(status int) {
	c.exitStatus = status
}

// noop for runc
func (c *runcContainer) Delete() error {
	return nil
}

func (c *runcContainer) Processes() ([]runtime.Process, error) {
	procs := []runtime.Process{
		c.initProcess,
	}
	for _, p := range c.processes {
		procs = append(procs, p)
	}
	return procs, nil
}

func (c *runcContainer) RemoveProcess(pid int) error {
	if _, ok := c.processes[pid]; !ok {
		return runtime.ErrNotChildProcess
	}
	delete(c.processes, pid)
	return nil
}

func (c *runcContainer) State() runtime.State {
	// TODO: how to do this with runc
	return runtime.State{
		Status: runtime.Running,
	}
}

func (c *runcContainer) Resume() error {
	return c.newCommand("resume").Run()
}

func (c *runcContainer) Pause() error {
	return c.newCommand("pause").Run()
}

// TODO: pass arguments
func (c *runcContainer) Checkpoint(runtime.Checkpoint) error {
	return c.newCommand("checkpoint").Run()
}

// TODO: pass arguments
func (c *runcContainer) Restore(cp string) error {
	return c.newCommand("restore").Run()
}

// TODO: pass arguments
func (c *runcContainer) DeleteCheckpoint(cp string) error {
	return errors.New("not implemented")
}

// TODO: implement in runc
func (c *runcContainer) Checkpoints() ([]runtime.Checkpoint, error) {
	return nil, errors.New("not implemented")
}

func (c *runcContainer) newCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("runc", append([]string{"--root", c.stateDir, "--id", c.id}, args...)...)
	cmd.Dir = c.path
	return cmd
}

type runcProcess struct {
	cmd  *exec.Cmd
	spec specs.Process
}

// pid of the container, not of runc
func (p *runcProcess) Pid() (int, error) {
	return p.cmd.Process.Pid, nil
}

func (p *runcProcess) Spec() specs.Process {
	return p.spec
}

func (p *runcProcess) Signal(s os.Signal) error {
	return p.cmd.Process.Signal(s)
}

type runcRuntime struct {
	stateDir string
}

func (r *runcRuntime) Type() string {
	return "runc"
}

func (r *runcRuntime) Create(id, bundlePath string, stdio *runtime.Stdio) (runtime.Container, error) {
	cmd := exec.Command("runc", "--root", r.stateDir, "--id", id, "start")
	cmd.Dir = bundlePath
	//	cmd.Stderr = stdio.Stderr
	//	cmd.Stdout = stdio.Stdout
	var s specs.Spec
	f, err := os.Open(filepath.Join(bundlePath, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &runcContainer{
		id:       id,
		path:     bundlePath,
		stateDir: r.stateDir,
		initProcess: &runcProcess{
			cmd:  cmd,
			spec: s.Process,
		},
		processes: make(map[int]*runcProcess),
	}, nil
}

func (r *runcRuntime) StartProcess(ci runtime.Container, p specs.Process, stdio *runtime.Stdio) (runtime.Process, error) {
	c, ok := ci.(*runcContainer)
	if !ok {
		return nil, runtime.ErrInvalidContainerType
	}
	f, err := ioutil.TempFile("", "containerd")
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(f).Encode(p); err != nil {
		f.Close()
		return nil, err
	}
	cmd := c.newCommand("exec", f.Name())
	f.Close()
	process := &runcProcess{
		cmd:  cmd,
		spec: p,
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid, err := process.Pid()
	if err != nil {
		return nil, err
	}
	c.processes[pid] = process
	return process, nil
}
