package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/docker/containerd/api/grpc/types"
	"golang.org/x/net/context"
)

func (cs *ContainerdSuite) ListRunningContainers() ([]*types.Container, error) {
	resp, err := cs.grpcClient.State(context.Background(), &types.StateRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Containers, nil
}

func (cs *ContainerdSuite) SignalContainerProcess(id string, procId string, sig uint32) error {
	_, err := cs.grpcClient.Signal(context.Background(), &types.SignalRequest{
		Id:     id,
		Pid:    procId,
		Signal: sig,
	})
	return err
}

func (cs *ContainerdSuite) SignalContainer(id string, sig uint32) error {
	return cs.SignalContainerProcess(id, "init", sig)
}

func (cs *ContainerdSuite) KillContainer(id string) error {
	return cs.SignalContainerProcess(id, "init", uint32(syscall.SIGKILL))
}

func (cs *ContainerdSuite) PauseContainer(id string) error {
	_, err := cs.grpcClient.UpdateContainer(context.Background(), &types.UpdateContainerRequest{
		Id:     id,
		Pid:    "init",
		Status: "paused",
	})
	return err
}

func (cs *ContainerdSuite) ResumeContainer(id string) error {
	_, err := cs.grpcClient.UpdateContainer(context.Background(), &types.UpdateContainerRequest{
		Id:     id,
		Pid:    "init",
		Status: "running",
	})
	return err
}

func (cs *ContainerdSuite) GetContainerStats(id string) (*types.StatsResponse, error) {
	stats, err := cs.grpcClient.Stats(context.Background(), &types.StatsRequest{
		Id: id,
	})
	return stats, err
}

type stdio struct {
	stdin        string
	stdout       string
	stderr       string
	stdinf       *os.File
	stdoutf      *os.File
	stderrf      *os.File
	stdoutBuffer bytes.Buffer
	stderrBuffer bytes.Buffer
}

type containerProcess struct {
	containerId string
	pid         string
	bundle      *Bundle
	io          stdio
	eventsCh    chan *types.Event
	cs          *ContainerdSuite
	hasExited   bool
}

func (c *containerProcess) openIo() (err error) {
	defer func() {
		if err != nil {
			c.Cleanup()
		}
	}()

	c.io.stdinf, err = os.OpenFile(c.io.stdin, os.O_RDWR, 0)
	if err != nil {
		return err
	}

	c.io.stdoutf, err = os.OpenFile(c.io.stdout, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(&c.io.stdoutBuffer, c.io.stdoutf)

	c.io.stderrf, err = os.OpenFile(c.io.stderr, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	go io.Copy(&c.io.stderrBuffer, c.io.stderrf)

	return nil
}

func (c *containerProcess) GetEventsChannel() chan *types.Event {
	return c.eventsCh
}

func (c *containerProcess) GetNextEvent() *types.Event {
	if c.hasExited {
		return nil
	}

	e := <-c.eventsCh

	if e.Type == "exit" && e.Pid == c.pid {
		c.Cleanup()
		c.hasExited = true
		close(c.eventsCh)
	}

	return e
}

func (c *containerProcess) CloseStdin() error {
	_, err := c.cs.grpcClient.UpdateProcess(context.Background(), &types.UpdateProcessRequest{
		Id:         c.containerId,
		Pid:        c.pid,
		CloseStdin: true,
	})
	return err
}

func (c *containerProcess) Cleanup() {
	for _, f := range []*os.File{
		c.io.stdinf,
		c.io.stdoutf,
		c.io.stderrf,
	} {
		if f != nil {
			f.Close()
			f = nil
		}
	}
}

func NewContainerProcess(cs *ContainerdSuite, bundle *Bundle, cid, pid string) (c *containerProcess, err error) {
	c = &containerProcess{
		containerId: cid,
		pid:         "init",
		bundle:      bundle,
		eventsCh:    make(chan *types.Event, 8),
		cs:          cs,
		hasExited:   false,
	}

	for name, path := range map[string]*string{
		"stdin":  &c.io.stdin,
		"stdout": &c.io.stdout,
		"stderr": &c.io.stderr,
	} {
		*path = filepath.Join(bundle.Path, "io", cid+"-"+pid+"-"+name)
		if err = syscall.Mkfifo(*path, 0755); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}

	if err = c.openIo(); err != nil {
		return nil, err
	}

	return c, nil
}

func (cs *ContainerdSuite) StartContainerWithEventFilter(id, bundleName string, filter func(*types.Event)) (c *containerProcess, err error) {
	bundle := GetBundle(bundleName)
	if bundle == nil {
		return nil, fmt.Errorf("No such bundle '%s'", bundleName)
	}

	c, err = NewContainerProcess(cs, bundle, id, "init")
	if err != nil {
		return nil, err
	}

	r := &types.CreateContainerRequest{
		Id:         id,
		BundlePath: filepath.Join(cs.cwd, bundle.Path),
		Stdin:      filepath.Join(cs.cwd, c.io.stdin),
		Stdout:     filepath.Join(cs.cwd, c.io.stdout),
		Stderr:     filepath.Join(cs.cwd, c.io.stderr),
	}

	if filter == nil {
		filter = func(event *types.Event) {
			c.eventsCh <- event
		}
	}

	cs.SetContainerEventFilter(id, filter)

	if _, err := cs.grpcClient.CreateContainer(context.Background(), r); err != nil {
		c.Cleanup()
		return nil, err
	}

	return c, nil
}

func (cs *ContainerdSuite) StartContainer(id, bundleName string) (c *containerProcess, err error) {
	return cs.StartContainerWithEventFilter(id, bundleName, nil)
}

func (cs *ContainerdSuite) RunContainer(id, bundleName string) (c *containerProcess, err error) {
	c, err = cs.StartContainer(id, bundleName)
	if err != nil {
		return nil, err
	}

	for {
		e := c.GetNextEvent()
		if e.Type == "exit" && e.Pid == "init" {
			break
		}
	}

	return c, err
}

func (cs *ContainerdSuite) AddProcessToContainer(init *containerProcess, pid, cwd string, env, args []string, uid, gid uint32) (c *containerProcess, err error) {
	c, err = NewContainerProcess(cs, init.bundle, init.containerId, pid)
	if err != nil {
		return nil, err
	}

	pr := &types.AddProcessRequest{
		Id:   init.containerId,
		Pid:  pid,
		Args: args,
		Cwd:  cwd,
		Env:  env,
		User: &types.User{
			Uid: uid,
			Gid: gid,
		},
		Stdin:  filepath.Join(cs.cwd, c.io.stdin),
		Stdout: filepath.Join(cs.cwd, c.io.stdout),
		Stderr: filepath.Join(cs.cwd, c.io.stderr),
	}

	_, err = cs.grpcClient.AddProcess(context.Background(), pr)
	if err != nil {
		c.Cleanup()
		return nil, err
	}

	return c, nil
}
