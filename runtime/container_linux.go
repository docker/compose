package runtime

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
)

func getRootIDs(s *PlatformSpec) (int, int, error) {
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

func (c *container) Pause() error {
	return exec.Command("runc", "pause", c.id).Run()
}

func (c *container) Resume() error {
	return exec.Command("runc", "resume", c.id).Run()
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

func (c *container) Start(checkpoint string, s Stdio) (Process, error) {
	processRoot := filepath.Join(c.root, c.id, InitProcessID)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command("containerd-shim",
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
		checkpoint:  checkpoint,
		root:        processRoot,
		id:          InitProcessID,
		c:           c,
		stdio:       s,
		spec:        spec,
		processSpec: ProcessSpec(spec.Process),
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

func (c *container) Exec(pid string, spec ProcessSpec, s Stdio) (Process, error) {
	processRoot := filepath.Join(c.root, c.id, pid)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command("containerd-shim",
		c.id, c.bundle,
	)
	cmd.Dir = processRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	config := &processConfig{
		exec:        true,
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

func (c *container) getLibctContainer() (libcontainer.Container, error) {
	f, err := libcontainer.New(specs.LinuxStateDirectory, libcontainer.Cgroupfs)
	if err != nil {
		return nil, err
	}
	return f.Load(c.id)
}

func hostIDFromMap(id uint32, mp []specs.IDMapping) int {
	for _, m := range mp {
		if (id >= m.ContainerID) && (id <= (m.ContainerID + m.Size - 1)) {
			return int(m.HostID + (id - m.ContainerID))
		}
	}
	return 0
}

func (c *container) Pids() ([]int, error) {
	container, err := c.getLibctContainer()
	if err != nil {
		return nil, err
	}
	return container.Processes()
}

func (c *container) Stats() (*Stat, error) {
	container, err := c.getLibctContainer()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	stats, err := container.Stats()
	if err != nil {
		return nil, err
	}
	return &Stat{
		Timestamp: now,
		Data:      stats,
	}, nil
}
