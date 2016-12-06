package oci

import (
	"errors"
	"fmt"
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
		runc: &runc.Runc{
			Root: filepath.Join(root, "runc"),
		},
	}
}

type OCIRuntime struct {
	// root holds runtime state information for the containers
	root string
	runc *runc.Runc
}

func (r *OCIRuntime) Create(id string, o execution.CreateOpts) (container *execution.Container, err error) {
	if container, err = execution.NewContainer(r.root, id, o.Bundle); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			container.StateDir().Delete()
		}
	}()
	initDir, err := container.StateDir().NewProcess()
	if err != nil {
		return nil, err
	}
	pidFile := filepath.Join(initDir, "pid")
	err = r.runc.Create(id, o.Bundle, &runc.CreateOpts{
		PidFile: pidFile,
		IO: runc.IO{
			Stdin:  o.Stdin,
			Stdout: o.Stdout,
			Stderr: o.Stderr,
		},
	})
	if err != nil {
		return nil, err
	}
	pid, err := runc.ReadPidFile(pidFile)
	if err != nil {
		// TODO: kill the container if we are going to return
		return nil, err
	}
	process, err := newProcess(filepath.Base(initDir), pid)
	if err != nil {
		return nil, err
	}

	container.AddProcess(process, true)

	return container, nil
}

func (r *OCIRuntime) Start(c *execution.Container) error {
	return r.runc.Start(c.ID())
}

func (r *OCIRuntime) Status(c *execution.Container) (execution.Status, error) {
	state, err := r.runc.State(c.ID())
	if err != nil {
		return "", err
	}
	return execution.Status(state.Status), nil
}

func (r *OCIRuntime) load(runcC *runc.Container) (*execution.Container, error) {
	container := execution.LoadContainer(
		execution.StateDir(filepath.Join(r.root, runcC.ID)),
		runcC.ID,
		runcC.Bundle,
		int64(runcC.Pid),
	)

	dirs, err := container.StateDir().Processes()
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		pid, err := runc.ReadPidFile(filepath.Join(d, "pid"))
		if err != nil {
			return nil, err
		}
		process, err := newProcess(filepath.Base(d), pid)
		if err != nil {
			return nil, err
		}
		container.AddProcess(process, pid == runcC.Pid)
	}

	return container, nil
}

func (r *OCIRuntime) List() ([]*execution.Container, error) {
	runcCs, err := r.runc.List()
	if err != nil {
		return nil, err
	}

	var containers []*execution.Container
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
	if err := r.runc.Delete(c.ID()); err != nil {
		return err
	}
	c.StateDir().Delete()
	return nil
}

func (r *OCIRuntime) Pause(c *execution.Container) error {
	return r.runc.Pause(c.ID())
}

func (r *OCIRuntime) Resume(c *execution.Container) error {
	return r.runc.Resume(c.ID())
}

func (r *OCIRuntime) StartProcess(c *execution.Container, o execution.CreateProcessOpts) (p execution.Process, err error) {
	processStateDir, err := c.StateDir().NewProcess()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.StateDir().DeleteProcess(filepath.Base(processStateDir))
		}
	}()

	pidFile := filepath.Join(processStateDir, "pid")
	if err := r.runc.ExecProcess(c.ID(), o.Spec, &runc.ExecOpts{
		PidFile: pidFile,
		Detach:  true,
		IO: runc.IO{
			Stdin:  o.Stdin,
			Stdout: o.Stdout,
			Stderr: o.Stderr,
		},
	}); err != nil {
		return nil, err
	}
	pid, err := runc.ReadPidFile(pidFile)
	if err != nil {
		return nil, err
	}

	process, err := newProcess(filepath.Base(processStateDir), pid)
	if err != nil {
		return nil, err
	}

	c.AddProcess(process, false)

	return process, nil
}

func (r *OCIRuntime) SignalProcess(c *execution.Container, id string, sig os.Signal) error {
	process := c.GetProcess(id)
	if process == nil {
		return fmt.Errorf("Make a Process Not Found error")
	}
	return syscall.Kill(int(process.Pid()), sig.(syscall.Signal))
}

func (r *OCIRuntime) DeleteProcess(c *execution.Container, id string) error {
	return c.StateDir().DeleteProcess(id)
}
