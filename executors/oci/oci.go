package oci

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/docker/containerd"
	"github.com/docker/containerd/executors"
)

var ErrRootEmpty = errors.New("oci: runtime root cannot be an empty string")

func init() {
	executors.Register("oci", New)
	executors.Register("runc", New)
}

func New() *OCIRuntime {
	return &OCIRuntime{
		root: opts.Root,
		name: opts.Name,
		args: opts.Args,
	}
}

type OCIRuntime struct {
	// root holds runtime state information for the containers
	// launched by the runtime
	root string
	// name is the name of the runtime, i.e. runc
	name string
	// args specifies additional arguments to the OCI runtime
	args []string
}

func (r *OCIRuntime) Name() string {
	return r.name
}

func (r *OCIRuntime) Args() []string {
	return r.args
}

func (r *OCIRuntime) Root() string {
	return r.root
}

func (r *OCIRuntime) Create(c *containerd.Container) (containerd.ProcessDelegate, error) {
	pidFile := fmt.Sprintf("%s/%s.pid", filepath.Join(r.root, c.ID()), "init")
	cmd := r.Command("create", "--pid-file", pidFile, "--bundle", c.Path(), c.ID())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.Stdin, c.Stdout, c.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return nil, err
	}
	i, err := strconv.Atoi(string(data))
	if err != nil {
		return nil, err
	}
	return newProcess(i)
}

func (r *OCIRuntime) Start(c *containerd.Container) error {
	return r.Command("start", c.ID()).Run()
}

func (r *OCIRuntime) Delete(c *containerd.Container) error {
	return r.Command("delete", c.ID()).Run()
}

func (r *OCIRuntime) Exec(c *containerd.Container, p *containerd.Process) (containerd.ProcessDelegate, error) {
	f, err := ioutil.TempFile(filepath.Join(r.root, c.ID()), "process")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	pidFile := fmt.Sprintf("%s/%s.pid", filepath.Join(r.root, c.ID()), filepath.Base(path))
	err = json.NewEncoder(f).Encode(p.Spec())
	f.Close()
	if err != nil {
		return nil, err
	}
	cmd := r.Command("exec", "--detach", "--process", path, "--pid-file", pidFile, c.ID())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.Stdin, p.Stdout, p.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return nil, err
	}
	i, err := strconv.Atoi(string(data))
	if err != nil {
		return nil, err
	}
	return newProcess(i)
}

type state struct {
	ID          string            `json:"id"`
	Pid         int               `json:"pid"`
	Status      string            `json:"status"`
	Bundle      string            `json:"bundle"`
	Rootfs      string            `json:"rootfs"`
	Created     time.Time         `json:"created"`
	Annotations map[string]string `json:"annotations"`
}

func (r *OCIRuntime) Load(id string) (containerd.ProcessDelegate, error) {
	data, err := r.Command("state", id).Output()
	if err != nil {
		return nil, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return newProcess(s.Pid)
}

func (r *OCIRuntime) Command(args ...string) *exec.Cmd {
	baseArgs := append([]string{
		"--root", r.root,
	}, r.args...)
	return exec.Command(r.name, append(baseArgs, args...)...)
}
