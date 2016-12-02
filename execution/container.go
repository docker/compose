package execution

import (
	"fmt"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type ContainerController interface {
	Pause(*Container) error
	Resume(*Container) error
	Status(*Container) (Status, error)
	Process(c *Container, pid int) (*Process, error)
	Processes(*Container) ([]*Process, error)
}

func NewContainer(c ContainerController) *Container {
	return &Container{
		controller: c,
	}
}

type Container struct {
	ID         string
	Bundle     string
	Root       string
	controller ContainerController

	processes map[int]*Process
}

func (c *Container) Process(pid int) (*Process, error) {
	for _, p := range c.processes {
		if p.Pid == pid {
			return p, nil
		}
	}
	return nil, fmt.Errorf("todo make real error")
}

func (c *Container) CreateProcess(spec *specs.Process) (*Process, error) {
	if err := os.MkdirAll(filepath.Join(c.Root, c.getNextProcessID()), 0660); err != nil {
		return nil, err
	}
	process := &Process{
		Spec:       spec,
		controller: c.controller,
	}
	c.processes = append(c.processes, process)
	return process, nil
}

func (c *Container) DeleteProcess(pid int) error {
	process, ok := c.processes[pid]
	if !ok {
		return fmt.Errorf("it no here")
	}
	if process.Status() != Stopped {
		return fmt.Errorf("tototoit not stopped ok?")
	}
	delete(c.processes, pid)
	return os.RemoveAll(p.Root)
}

func (c *Container) Processes() []*Process {
	var out []*Process
	for _, p := range c.processes {
		out = append(out, p)
	}
	return out
}

func (c *Container) Pause() error {
	return c.controller.Pause(c)
}

func (c *Container) Resume() error {
	return c.controller.Resume(c)
}

func (c *Container) Status() (Status, error) {
	return c.controller.Status(c)
}
