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
	"github.com/docker/containerd/execution"
	"github.com/docker/containerd/executors"
)

var ErrRootEmpty = errors.New("oci: runtime root cannot be an empty string")

func init() {
	executors.Register("oci", New)
	executors.Register("runc", New)
}

func New(root string) *OCIRuntime {
	return &OCIRuntime{
		root: root,
	}
}

type OCIRuntime struct {
	// root holds runtime state information for the containers
	// launched by the runtime
	root string
}

func (r *OCIRuntime) Create(id string, o execution.CreateOpts) (*execution.Container, error) {
	// /run/runc/redis/1/pid
	pidFile := filepath.Join(r.root, id, "1", "pid")
	cmd := command(r.root, "create",
		"--pid-file", pidFile,
		"--bundle", o.Bundle,
		id,
	)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	// TODO: kill on error
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return nil, err
	}
	container := execution.NewContainer(r)
	container.ID = id
	container.Root = filepath.Join(r.root, id)
	container.Bundle = o.Bundle
	process, err := container.CreateProcess(nil)
	if err != nil {
		return nil, err
	}
	process.Pid = pid
	process.Stdin = o.Stdin
	process.Stdout = o.Stdout
	process.Stderr = o.Stderr
	return container, nil
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

func (r *OCIRuntime) Delete(id string) error {
	return command(r.root, "delete", id).Run()
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

func command(root, args ...string) *exec.Cmd {
	return exec.Command("runc", append(
		[]string{"--root", root},
		args...)...)
}
