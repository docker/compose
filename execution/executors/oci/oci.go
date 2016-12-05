package oci

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/crosbymichael/go-runc"
	"github.com/docker/containerd/execution"
)

var ErrRootEmpty = errors.New("oci: runtime root cannot be an empty string")

func New(root string) *OCIRuntime {
	return &OCIRuntime{
		root: root,
		Runc: &runc.Runc{
			Root: filepath.Join(root, "runc"),
		},
	}
}

type OCIRuntime struct {
	// root holds runtime state information for the containers
	root string
	runc *runc.Runc
}

func (r *OCIRuntime) Create(id string, o execution.CreateOpts) (*execution.Container, error) {
	var err error

	stateDir, err := NewStateDir(r.root, id)
	if err != nil {
		return nil, err
	}

	initStateDir, err := stateDir.NewProcess()
	if err != nil {
		return nil, err
	}

	// /run/runc/redis/1/pid
	pidFile := filepath.Join(initStateDir, "pid")
	err = r.runc.Create(id, o.Bundle, &runc.CreateOpts{
		Pidfile: pidfile,
		Stdin:   o.Stdin,
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
	})
	if err != nil {
		return nil, err
	}
	pid, err := runc.ReadPifFile(pidfile)
	if err != nil {
		return nil, err
	}
	process, err := newProcess(pid)
	if err != nil {
		return nil, err
	}

	container := &execution.Container{
		ID:       id,
		Bundle:   o.Bundle,
		StateDir: stateDir,
	}
	container.AddProcess(process)

	return container, nil
}

func (r *OCIRuntime) load(runcC *runc.Container) (*execution.Container, error) {
	container := &execution.Container{
		ID:       runcC.ID,
		Bundle:   runcC.Bundle,
		StateDir: StateDir(filepath.Join(r.root, runcC.ID)),
	}

	process, err := newProcess(runcC.Pid)
	if err != nil {
		return nil, err
	}
	container.AddProcess(process)

	// /run/containerd/container-id/processess/process-id
	dirs, err := ioutil.ReadDir(filepath.Join(container.Root))
	if err != nil {
		return nil, err
	}

	return container, nil
}

func (r *OCIRuntime) List() ([]*execution.Container, error) {
	runcCs, err := r.runc.List()
	if err != nil {
		return nil, err
	}

	containers := make([]*execution.Container)
	for _, c := range runcCs {
		container, err := r.load(c)
		if err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}

	return containers, nil
}

func (r *OCIRuntime) Load(id string) (*execution.Container, error) {
	runcC, err := r.runc.State(id)
	if err != nil {
		return nil, err
	}

	return r.load(runcC)
}

func (r *OCIRuntime) Delete(c *execution.Container) error {
	if err := r.runc.Delete(c.ID); err != nil {
		return err
	}
	c.StateDir.Delete()
	return nil
}

func (r *OCIRuntime) Pause(c *execution.Container) error {
	return r.runc.Pause(c.ID)
}

func (r *OCIRuntime) Resume(c *execution.Container) error {
	return r.runc.Resume(c.ID)
}

func (r *OCIRuntime) StartProcess(c *execution.Container, o CreateProcessOpts) (execution.Process, error) {
	var err error

	processStateDir, err := c.StateDir.NewProcess()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.StateDir.DeleteProcess(filepath.Base(processStateDir))
		}
	}()

	pidFile := filepath.Join(processStateDir, id)
	err := r.runc.ExecProcess(c.ID, o.spec, &runc.ExecOpts{
		PidFile: pidfile,
		Detach:  true,
		Stdin:   o.stdin,
		Stdout:  o.stdout,
		Stderr:  o.stderr,
	})
	if err != nil {
		return nil, err
	}
	pid, err := runc.ReadPidFile(pidfile)
	if err != nil {
		return nil, err
	}

	process, err := newProcess(pid)
	if err != nil {
		return nil, err
	}

	container.AddProcess(process)

	return process, nil
}

func (r *OCIRuntime) SignalProcess(c *execution.Container, id string, sig os.Signal) error {
	process := c.GetProcess(id)
	if process == nil {
		return fmt.Errorf("Make a Process Not Found error")
	}
	return syscall.Kill(int(process.Pid()), os.Signal)
}

func (r *OCIRuntime) GetProcess(c *execution.Container, id string) process {
	return c.GetProcess(id)
}

func (r *OCIRuntime) DeleteProcess(c *execution.Container, id string) error {
	c.StateDir.DeleteProcess(id)
	return nil
}
