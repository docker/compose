package runtime

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/opencontainers/specs"
)

type Container interface {
	// ID returns the container ID
	ID() string
	// Path returns the path to the bundle
	Path() string
	// Start starts the init process of the container
	Start(checkpoint string, s Stdio) (Process, error)
	// Exec starts another process in an existing container
	Exec(string, specs.Process, Stdio) (Process, error)
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
	// Stats returns realtime container stats and resource information
	// Stats() (*Stat, error) // OOM signals the channel if the container received an OOM notification
	// OOM() (<-chan struct{}, error)
}

type Stdio struct {
	Stdin  string
	Stdout string
	Stderr string
}

// New returns a new container
func New(root, id, bundle string) (Container, error) {
	c := &container{
		root:      root,
		id:        id,
		bundle:    bundle,
		processes: make(map[string]*process),
	}
	if err := os.Mkdir(filepath.Join(root, id), 0755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(root, id, StateFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(state{
		Bundle: bundle,
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
		root:      root,
		id:        id,
		bundle:    s.Bundle,
		processes: make(map[string]*process),
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
	root      string
	id        string
	bundle    string
	processes map[string]*process
	stdio     Stdio
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Path() string {
	return c.bundle
}

func (c *container) Start(checkpoint string, s Stdio) (Process, error) {
	processRoot := filepath.Join(c.root, c.id, InitProcessID)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command("containerd-shim",
		"-checkpoint", checkpoint,
		c.id, c.bundle,
	)
	cmd.Dir = processRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	spec, err := c.readSpec()
	if err != nil {
		return nil, err
	}
	config := &processConfig{
		root:        processRoot,
		id:          InitProcessID,
		c:           c,
		stdio:       s,
		spec:        spec,
		processSpec: spec.Process,
	}
	p, err := newProcess(config)
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if _, err := p.getPid(); err != nil {
		return p, nil
	}
	c.processes[InitProcessID] = p
	return p, nil
}

func (c *container) Exec(pid string, spec specs.Process, s Stdio) (Process, error) {
	processRoot := filepath.Join(c.root, c.id, pid)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command("containerd-shim",
		"-exec",
		c.id, c.bundle,
	)
	cmd.Dir = processRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	config := &processConfig{
		id:          pid,
		root:        processRoot,
		c:           c,
		processSpec: spec,
		stdio:       s,
	}
	p, err := newProcess(config)
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if _, err := p.getPid(); err != nil {
		return p, nil
	}
	c.processes[pid] = p
	return p, nil
}

func (c *container) readSpec() (*specs.LinuxSpec, error) {
	var spec specs.LinuxSpec
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

func (c *container) Pause() error {
	return exec.Command("runc", "pause", c.id).Run()
}

func (c *container) Resume() error {
	return exec.Command("runc", "resume", c.id).Run()
}

func (c *container) State() State {
	return Running
}

func (c *container) Delete() error {
	return os.RemoveAll(filepath.Join(c.root, c.id))
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

func (c *container) Checkpoints() ([]Checkpoint, error) {
	dirs, err := ioutil.ReadDir(filepath.Join(c.bundle, "checkpoints"))
	if err != nil {
		return nil, err
	}
	var out []Checkpoint
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		path := filepath.Join(c.bundle, "checkpoints", d.Name(), "config.json")
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var cpt Checkpoint
		if err := json.Unmarshal(data, &cpt); err != nil {
			return nil, err
		}
		out = append(out, cpt)
	}
	return out, nil
}

func (c *container) Checkpoint(cpt Checkpoint) error {
	if err := os.MkdirAll(filepath.Join(c.bundle, "checkpoints"), 0755); err != nil {
		return err
	}
	path := filepath.Join(c.bundle, "checkpoints", cpt.Name)
	if err := os.Mkdir(path, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(path, "config.json"))
	if err != nil {
		return err
	}
	cpt.Created = time.Now()
	err = json.NewEncoder(f).Encode(cpt)
	f.Close()
	if err != nil {
		return err
	}
	args := []string{
		"checkpoint",
		"--image-path", path,
	}
	add := func(flags ...string) {
		args = append(args, flags...)
	}
	if !cpt.Exit {
		add("--leave-running")
	}
	if cpt.Shell {
		add("--shell-job")
	}
	if cpt.Tcp {
		add("--tcp-established")
	}
	if cpt.UnixSockets {
		add("--ext-unix-sk")
	}
	add(c.id)
	return exec.Command("runc", args...).Run()
}

func (c *container) DeleteCheckpoint(name string) error {
	return os.RemoveAll(filepath.Join(c.bundle, "checkpoints", name))
}

func getRootIDs(s *specs.LinuxSpec) (int, int, error) {
	if s == nil {
		return 0, 0, nil
	}
	var hasUserns bool
	for _, ns := range s.Linux.Namespaces {
		if ns.Type == specs.UserNamespace {
			hasUserns = true
			break
		}
	}
	if !hasUserns {
		return 0, 0, nil
	}
	uid := hostIDFromMap(0, s.Linux.UIDMappings)
	gid := hostIDFromMap(0, s.Linux.GIDMappings)
	return uid, gid, nil
}

func hostIDFromMap(id uint32, mp []specs.IDMapping) int {
	for _, m := range mp {
		if (id >= m.ContainerID) && (id <= (m.ContainerID + m.Size - 1)) {
			return int(m.HostID + (id - m.ContainerID))
		}
	}
	return 0
}
