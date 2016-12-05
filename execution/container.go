package execution

type Container struct {
	ID       string
	Bundle   string
	StateDir StateDir

	processes map[string]Process
}

func (c *Container) AddProcess(p Process) {
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
