package runtime

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/specs"
)

type Container interface {
	// ID returns the container ID
	ID() string
	// Path returns the path to the bundle
	Path() string
	// Start starts the init process of the container
	Start(checkpoint string, s Stdio) (Process, error)
	// Exec starts another process in an existing container
	Exec(string, specs.ProcessSpec, Stdio) (Process, error)
	// Delete removes the container's state and any resources
	Delete() error
	// Processes returns all the containers processes that have been added
	Processes() ([]Process, error)
	// State returns the containers runtime state
	State() State
	// Resume resumes a paused container
	Resume() error
	// Pause pauses a running container
	Pause() error
	// RemoveProcess removes the specified process from the container
	RemoveProcess(string) error
	// Checkpoints returns all the checkpoints for a container
	Checkpoints() ([]Checkpoint, error)
	// Checkpoint creates a new checkpoint
	Checkpoint(Checkpoint) error
	// DeleteCheckpoint deletes the checkpoint for the provided name
	DeleteCheckpoint(name string) error
	// Labels are user provided labels for the container
	Labels() []string
	// Pids returns all pids inside the container
	Pids() ([]int, error)
	// Stats returns realtime container stats and resource information
	Stats() (*Stat, error)
	// Name or path of the OCI compliant runtime used to execute the container
	Runtime() string
	// OOM signals the channel if the container received an OOM notification
	OOM() (OOM, error)
	// UpdateResource updates the containers resources to new values
	UpdateResources(*Resource) error
}

type OOM interface {
	io.Closer
	FD() int
	ContainerID() string
	Flush()
	Removed() bool
}

type Stdio struct {
	Stdin  string
	Stdout string
	Stderr string
}

func NewStdio(stdin, stdout, stderr string) Stdio {
	for _, s := range []*string{
		&stdin, &stdout, &stderr,
	} {
		if *s == "" {
			*s = "/dev/null"
		}
	}
	return Stdio{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
}

type ContainerOpts struct {
	Root        string
	ID          string
	Bundle      string
	Runtime     string
	RuntimeArgs []string
	Shim        string
	Labels      []string
	NoPivotRoot bool
	Timeout     time.Duration
}

// New returns a new container
func New(opts ContainerOpts) (Container, error) {
	c := &container{
		root:        opts.Root,
		id:          opts.ID,
		bundle:      opts.Bundle,
		labels:      opts.Labels,
		processes:   make(map[string]*process),
		runtime:     opts.Runtime,
		runtimeArgs: opts.RuntimeArgs,
		shim:        opts.Shim,
		noPivotRoot: opts.NoPivotRoot,
		timeout:     opts.Timeout,
	}
	if err := os.Mkdir(filepath.Join(c.root, c.id), 0755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(c.root, c.id, StateFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(state{
		Bundle:      c.bundle,
		Labels:      c.labels,
		Runtime:     c.runtime,
		RuntimeArgs: c.runtimeArgs,
		NoPivotRoot: opts.NoPivotRoot,
	}); err != nil {
		return nil, err
	}
	return c, nil
}

func Load(root, id string) (Container, error) {
	var s state
	f, err := os.Open(filepath.Join(root, id, StateFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	c := &container{
		root:        root,
		id:          id,
		bundle:      s.Bundle,
		labels:      s.Labels,
		runtime:     s.Runtime,
		runtimeArgs: s.RuntimeArgs,
		shim:        s.Shim,
		noPivotRoot: s.NoPivotRoot,
		processes:   make(map[string]*process),
	}
	dirs, err := ioutil.ReadDir(filepath.Join(root, id))
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		pid := d.Name()
		s, err := readProcessState(filepath.Join(root, id, pid))
		if err != nil {
			return nil, err
		}
		p, err := loadProcess(filepath.Join(root, id, pid), pid, c, s)
		if err != nil {
			logrus.WithField("id", id).WithField("pid", pid).Debug("containerd: error loading process %s", err)
			continue
		}
		c.processes[pid] = p
	}
	return c, nil
}

func readProcessState(dir string) (*ProcessState, error) {
	f, err := os.Open(filepath.Join(dir, "process.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s ProcessState
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

type container struct {
	// path to store runtime state information
	root        string
	id          string
	bundle      string
	runtime     string
	runtimeArgs []string
	shim        string
	processes   map[string]*process
	labels      []string
	oomFds      []int
	noPivotRoot bool
	timeout     time.Duration
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Path() string {
	return c.bundle
}

func (c *container) Labels() []string {
	return c.labels
}

func (c *container) readSpec() (*specs.Spec, error) {
	var spec specs.Spec
	f, err := os.Open(filepath.Join(c.bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (c *container) Delete() error {
	err := os.RemoveAll(filepath.Join(c.root, c.id))

	args := c.runtimeArgs
	args = append(args, "delete", c.id)
	if derr := exec.Command(c.runtime, args...).Run(); err == nil {
		err = derr
	}
	return err
}

func (c *container) Processes() ([]Process, error) {
	out := []Process{}
	for _, p := range c.processes {
		out = append(out, p)
	}
	return out, nil
}

func (c *container) RemoveProcess(pid string) error {
	delete(c.processes, pid)
	return os.RemoveAll(filepath.Join(c.root, c.id, pid))
}

func (c *container) UpdateResources(r *Resource) error {
	container, err := c.getLibctContainer()
	if err != nil {
		return err
	}
	config := container.Config()
	config.Cgroups.Resources.CpuShares = r.CPUShares
	config.Cgroups.Resources.BlkioWeight = r.BlkioWeight
	config.Cgroups.Resources.CpuPeriod = r.CPUPeriod
	config.Cgroups.Resources.CpuQuota = r.CPUQuota
	config.Cgroups.Resources.CpusetCpus = r.CpusetCpus
	config.Cgroups.Resources.CpusetMems = r.CpusetMems
	config.Cgroups.Resources.KernelMemory = r.KernelMemory
	config.Cgroups.Resources.Memory = r.Memory
	config.Cgroups.Resources.MemoryReservation = r.MemoryReservation
	config.Cgroups.Resources.MemorySwap = r.MemorySwap
	return container.Set(config)
}
