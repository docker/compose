package runtime

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/opencontainers/specs"
)

const (
	ExitFile       = "exit"
	ExitStatusFile = "exitStatus"
	StateFile      = "state.json"
	InitProcessID  = "init"
)

type state struct {
	Bundle string `json:"bundle"`
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
		// TODO: get the process spec from a state file in the process dir
		p, err := loadProcess(filepath.Join(root, id, pid), pid, c, specs.Process{})
		if err != nil {
			if err == ErrProcessExited {
				logrus.WithField("id", id).WithField("pid", pid).Debug("containerd: process exited while away")
				// TODO: fire events to do the removal
				if err := os.RemoveAll(filepath.Join(root, id, pid)); err != nil {
					logrus.WithField("error", err).Warn("containerd: remove process state")
				}
				continue
			}
			return nil, err
		}
		c.processes[pid] = p
	}
	if len(c.processes) == 0 {
		return nil, ErrContainerExited
	}
	return c, nil
}

type container struct {
	// path to store runtime state information
	root      string
	id        string
	bundle    string
	processes map[string]*process
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Path() string {
	return c.bundle
}

func (c *container) Start() (Process, error) {
	processRoot := filepath.Join(c.root, c.id, InitProcessID)
	if err := os.MkdirAll(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command("containerd-shim", processRoot, c.id)
	cmd.Dir = c.bundle
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	spec, err := c.readSpec()
	if err != nil {
		return nil, err
	}
	p, err := newProcess(processRoot, InitProcessID, c, spec.Process)
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.processes[InitProcessID] = p
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
	return errNotImplemented
}

func (c *container) Resume() error {
	return errNotImplemented
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
