package execution

import "fmt"

func NewContainer(stateRoot, id, bundle string) (*Container, error) {
	stateDir, err := NewStateDir(stateRoot, id)
	if err != nil {
		return nil, err
	}
	return &Container{
		id:        id,
		bundle:    bundle,
		stateDir:  stateDir,
		status:    Created,
		processes: make(map[string]Process),
	}, nil
}

func LoadContainer(dir StateDir, id, bundle string, status Status, initPid int64) *Container {
	return &Container{
		id:        id,
		stateDir:  dir,
		bundle:    bundle,
		initPid:   initPid,
		status:    status,
		processes: make(map[string]Process),
	}
}

type Container struct {
	id       string
	bundle   string
	stateDir StateDir
	initPid  int64
	status   Status

	processes map[string]Process
}

func (c *Container) ID() string {
	return c.id
}

func (c *Container) Status() Status {
	return c.status
}

func (c *Container) Bundle() string {
	return c.bundle
}

func (c *Container) StateDir() StateDir {
	return c.stateDir
}

func (c *Container) Wait() (uint32, error) {
	for _, p := range c.processes {
		if p.Pid() == c.initPid {
			return p.Wait()
		}
	}
	return 0, fmt.Errorf("no init process")
}

func (c *Container) AddProcess(p Process, isInit bool) {
	if isInit {
		c.initPid = p.Pid()
	}
	c.processes[p.ID()] = p
}

func (c *Container) GetProcess(id string) Process {
	return c.processes[id]
}

func (c *Container) RemoveProcess(id string) {
	delete(c.processes, id)
}

func (c *Container) Processes() []Process {
	var out []Process
	for _, p := range c.processes {
		out = append(out, p)
	}
	return out
}
