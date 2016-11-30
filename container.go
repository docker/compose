package containerd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type ContainerConfig interface {
	ID() string
	Root() string // bundle path
	Spec() (*specs.Spec, error)
	Runtime() (Runtime, error)
}

func NewContainer(config ContainerConfig) (*Container, error) {
	var (
		id   = config.ID()
		root = config.Root()
		path = filepath.Join(root, id)
	)
	s, err := config.Spec()
	if err != nil {
		return nil, err
	}
	// HACK: for runc to allow to use this path without a premounted rootfs
	if err := os.MkdirAll(filepath.Join(path, s.Root.Path), 0711); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(path, "config.json"))
	if err != nil {
		return nil, err
	}
	// write the spec file to the container's directory
	err = json.NewEncoder(f).Encode(s)
	f.Close()
	if err != nil {
		return nil, err
	}
	r, err := config.Runtime()
	if err != nil {
		return nil, err
	}
	return &Container{
		id:     id,
		path:   path,
		s:      s,
		driver: r,
	}, nil
}

func LoadContainer(config ContainerConfig) (*Container, error) {
	var (
		id   = config.ID()
		root = config.Root()
		path = filepath.Join(root, id)
	)
	spec, err := loadSpec(path)
	if err != nil {
		return nil, err
	}
	r, err := config.Runtime()
	if err != nil {
		return nil, err
	}
	process, err := r.Load(id)
	if err != nil {
		return nil, err
	}
	// TODO: load exec processes
	return &Container{
		id:     id,
		path:   path,
		s:      spec,
		driver: r,
		init: &Process{
			d:      process,
			driver: r,
		},
	}, nil
}

func loadSpec(path string) (*specs.Spec, error) {
	f, err := os.Open(filepath.Join(path, "config.json"))
	if err != nil {
		return nil, err
	}
	var s specs.Spec
	err = json.NewDecoder(f).Decode(&s)
	f.Close()
	if err != nil {
		return nil, err
	}
	return &s, nil
}

type Container struct {
	mu   sync.Mutex
	id   string
	path string
	s    *specs.Spec

	driver Runtime

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// init is the container's init processes
	init *Process
	// processes is a list of additional processes executed inside the container
	// via the NewProcess method on the container
	processes []*Process
}

// ID returns the id of the container
func (c *Container) ID() string {
	return c.id
}

// Path returns the fully qualified path to the container on disk
func (c *Container) Path() string {
	return c.path
}

// Spec returns the OCI runtime spec for the container
func (c *Container) Spec() *specs.Spec {
	return c.s
}

// Create will create the container on the system by running the runtime's
// initial setup and process waiting for the user process to be started
func (c *Container) Create() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, err := c.driver.Create(c)
	if err != nil {
		return err
	}
	c.init = &Process{
		d:      d,
		driver: c.driver,
		Stdin:  c.Stdin,
		Stdout: c.Stdout,
		Stderr: c.Stderr,
	}
	return nil
}

// Start will start the container's user specified process
func (c *Container) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.driver.Start(c)
}

// NewProcess will create a new process that will be executed inside the
// container and tied to the init processes lifecycle
func (c *Container) NewProcess(spec *specs.Process) (*Process, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	process := &Process{
		s:      spec,
		c:      c,
		exec:   true,
		driver: c.driver,
	}
	c.processes = append(c.processes, process)
	return process, nil
}

// Pid returns the pid of the init or main process hosted inside the container
func (c *Container) Pid() int {
	c.mu.Lock()
	if c.init == nil {
		c.mu.Unlock()
		return -1
	}
	pid := c.init.Pid()
	c.mu.Unlock()
	return pid
}

// Wait will perform a blocking wait on the init process of the container
func (c *Container) Wait() (uint32, error) {
	c.mu.Lock()
	proc := c.init
	c.mu.Unlock()
	return proc.Wait()
}

// Signal will send the provided signal to the init process of the container
func (c *Container) Signal(s os.Signal) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.init.Signal(s)
}

// Delete will delete the container if it no long has any processes running
// inside the container and removes all state on disk for the container
func (c *Container) Delete() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.driver.Delete(c)
	if rerr := os.RemoveAll(c.path); err == nil {
		err = rerr
	}
	return err
}
