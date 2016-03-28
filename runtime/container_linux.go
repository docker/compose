package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docker/containerd/specs"
	"github.com/opencontainers/runc/libcontainer"
	ocs "github.com/opencontainers/specs/specs-go"
)

var shimBinary = os.Args[0] + "-shim"

func getRootIDs(s *specs.Spec) (int, int, error) {
	if s == nil {
		return 0, 0, nil
	}
	var hasUserns bool
	for _, ns := range s.Linux.Namespaces {
		if ns.Type == ocs.UserNamespace {
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

func (c *container) State() State {
	proc := c.processes["init"]
	if proc == nil {
		return Stopped
	}
	return proc.State()
}

func (c *container) Runtime() string {
	return c.runtime
}

func (c *container) Pause() error {
	args := c.runtimeArgs
	args = append(args, "pause", c.id)
	b, err := exec.Command(c.runtime, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(b))
	}
	return nil
}

func (c *container) Resume() error {
	args := c.runtimeArgs
	args = append(args, "resume", c.id)
	b, err := exec.Command(c.runtime, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(b))
	}
	return nil
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
	add(c.runtimeArgs...)
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
	return exec.Command(c.runtime, args...).Run()
}

func (c *container) DeleteCheckpoint(name string) error {
	return os.RemoveAll(filepath.Join(c.bundle, "checkpoints", name))
}

func (c *container) Start(checkpoint string, s Stdio) (Process, error) {
	processRoot := filepath.Join(c.root, c.id, InitProcessID)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	cmd := exec.Command(shimBinary,
		c.id, c.bundle, c.runtime,
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
		processSpec: specs.ProcessSpec(spec.Process),
	}
	p, err := newProcess(config)
	if err != nil {
		return nil, err
	}
	if err := c.startCmd(InitProcessID, cmd, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (c *container) Exec(pid string, pspec specs.ProcessSpec, s Stdio) (pp Process, err error) {
	processRoot := filepath.Join(c.root, c.id, pid)
	if err := os.Mkdir(processRoot, 0755); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			c.RemoveProcess(pid)
		}
	}()
	cmd := exec.Command(shimBinary,
		c.id, c.bundle, c.runtime,
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
		exec:        true,
		id:          pid,
		root:        processRoot,
		c:           c,
		processSpec: pspec,
		spec:        spec,
		stdio:       s,
	}
	p, err := newProcess(config)
	if err != nil {
		return nil, err
	}
	if err := c.startCmd(pid, cmd, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (c *container) startCmd(pid string, cmd *exec.Cmd, p *process) error {
	if err := cmd.Start(); err != nil {
		if exErr, ok := err.(*exec.Error); ok {
			if exErr.Err == exec.ErrNotFound || exErr.Err == os.ErrNotExist {
				return fmt.Errorf("%s not installed on system", shimBinary)
			}
		}
		return err
	}
	if err := c.waitForStart(p, cmd); err != nil {
		return err
	}
	c.processes[pid] = p
	return nil
}

func (c *container) getLibctContainer() (libcontainer.Container, error) {
	runtimeRoot := "/run/runc"

	// Check that the root wasn't changed
	for _, opt := range c.runtimeArgs {
		if strings.HasPrefix(opt, "--root=") {
			runtimeRoot = strings.TrimPrefix(opt, "--root=")
			break
		}
	}

	f, err := libcontainer.New(runtimeRoot, libcontainer.Cgroupfs)
	if err != nil {
		return nil, err
	}
	return f.Load(c.id)
}

func hostIDFromMap(id uint32, mp []ocs.IDMapping) int {
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

func (c *container) OOM() (OOM, error) {
	container, err := c.getLibctContainer()
	if err != nil {
		if lerr, ok := err.(libcontainer.Error); ok {
			// with oom registration sometimes the container can run, exit, and be destroyed
			// faster than we can get the state back so we can just ignore this
			if lerr.Code() == libcontainer.ContainerNotExists {
				return nil, ErrContainerExited
			}
		}
		return nil, err
	}
	state, err := container.State()
	if err != nil {
		return nil, err
	}
	memoryPath := state.CgroupPaths["memory"]
	return c.getMemeoryEventFD(memoryPath)
}

func (c *container) getMemeoryEventFD(root string) (*oom, error) {
	f, err := os.Open(filepath.Join(root, "memory.oom_control"))
	if err != nil {
		return nil, err
	}
	fd, _, serr := syscall.RawSyscall(syscall.SYS_EVENTFD2, 0, syscall.FD_CLOEXEC, 0)
	if serr != 0 {
		f.Close()
		return nil, serr
	}
	if err := c.writeEventFD(root, int(f.Fd()), int(fd)); err != nil {
		syscall.Close(int(fd))
		f.Close()
		return nil, err
	}
	return &oom{
		root:    root,
		id:      c.id,
		eventfd: int(fd),
		control: f,
	}, nil
}

func (c *container) writeEventFD(root string, cfd, efd int) error {
	f, err := os.OpenFile(filepath.Join(root, "cgroup.event_control"), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintf("%d %d", efd, cfd))
	return err
}

type waitArgs struct {
	pid int
	err error
}

func (c *container) waitForStart(p *process, cmd *exec.Cmd) error {
	wc := make(chan error, 1)
	go func() {
		for {
			if _, err := p.getPidFromFile(); err != nil {
				if os.IsNotExist(err) || err == errInvalidPidInt {
					alive, err := isAlive(cmd)
					if err != nil {
						wc <- err
						return
					}
					if !alive {
						// runc could have failed to run the container so lets get the error
						// out of the logs or the shim could have encountered an error
						messages, err := readLogMessages(filepath.Join(p.root, "shim-log.json"))
						if err != nil {
							wc <- err
							return
						}
						for _, m := range messages {
							if m.Level == "error" {
								wc <- fmt.Errorf("shim error: %v", m.Msg)
								return
							}
						}
						// no errors reported back from shim, check for runc/runtime errors
						messages, err = readLogMessages(filepath.Join(p.root, "log.json"))
						if err != nil {
							if os.IsNotExist(err) {
								err = ErrContainerNotStarted
							}
							wc <- err
							return
						}
						for _, m := range messages {
							if m.Level == "error" {
								wc <- fmt.Errorf("oci runtime error: %v", m.Msg)
								return
							}
						}
						wc <- ErrContainerNotStarted
						return
					}
					time.Sleep(50 * time.Millisecond)
					continue
				}
				wc <- err
				return
			}
			// the pid file was read successfully
			wc <- nil
			return
		}
	}()
	select {
	case err := <-wc:
		if err != nil {
			return err
		}
		return nil
	case <-time.After(c.timeout):
		cmd.Process.Kill()
		cmd.Wait()
		return ErrContainerStartTimeout
	}
}

// isAlive checks if the shim that launched the container is still alive
func isAlive(cmd *exec.Cmd) (bool, error) {
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		if err == syscall.ESRCH {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type oom struct {
	id      string
	root    string
	control *os.File
	eventfd int
}

func (o *oom) ContainerID() string {
	return o.id
}

func (o *oom) FD() int {
	return o.eventfd
}

func (o *oom) Flush() {
	buf := make([]byte, 8)
	syscall.Read(o.eventfd, buf)
}

func (o *oom) Removed() bool {
	_, err := os.Lstat(filepath.Join(o.root, "cgroup.event_control"))
	return os.IsNotExist(err)
}

func (o *oom) Close() error {
	err := syscall.Close(o.eventfd)
	if cerr := o.control.Close(); err == nil {
		err = cerr
	}
	return err
}

type message struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

func readLogMessages(path string) ([]message, error) {
	var out []message
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
