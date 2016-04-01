package main

import (
	"path/filepath"
	"syscall"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (cs *ContainerdSuite) TestBusyboxTopExecEcho(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	var (
		err   error
		initp *containerProcess
		echop *containerProcess
	)

	containerId := "top"
	initp, err = cs.StartContainer(containerId, bundleName)
	t.Assert(err, checker.Equals, nil)

	echop, err = cs.AddProcessToContainer(initp, "echo", "/", []string{"PATH=/bin"}, []string{"sh", "-c", "echo -n Ay Caramba! ; exit 1"}, 0, 0)
	t.Assert(err, checker.Equals, nil)

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerId,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "start-process",
			Id:     containerId,
			Status: 0,
			Pid:    "echo",
		},
		{
			Type:   "exit",
			Id:     containerId,
			Status: 1,
			Pid:    "echo",
		},
	} {
		ch := initp.GetEventsChannel()
		e := <-ch
		evt.Timestamp = e.Timestamp

		t.Assert(*e, checker.Equals, evt)
	}

	t.Assert(echop.io.stdoutBuffer.String(), checker.Equals, "Ay Caramba!")
}

func (cs *ContainerdSuite) TestBusyboxTopExecTop(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	var (
		err   error
		initp *containerProcess
	)

	containerId := "top"
	initp, err = cs.StartContainer(containerId, bundleName)
	t.Assert(err, checker.Equals, nil)

	execId := "top1"
	_, err = cs.AddProcessToContainer(initp, execId, "/", []string{"PATH=/usr/bin"}, []string{"top"}, 0, 0)
	t.Assert(err, checker.Equals, nil)

	for idx, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerId,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "start-process",
			Id:     containerId,
			Status: 0,
			Pid:    execId,
		},
		{
			Type:   "exit",
			Id:     containerId,
			Status: 137,
			Pid:    execId,
		},
	} {
		ch := initp.GetEventsChannel()
		e := <-ch
		evt.Timestamp = e.Timestamp
		t.Assert(*e, checker.Equals, evt)
		if idx == 1 {
			// Process Started, kill it
			cs.SignalContainerProcess(containerId, "top1", uint32(syscall.SIGKILL))
		}
	}

	// Container should still be running
	containers, err := cs.ListRunningContainers()
	if err != nil {
		t.Fatal(err)
	}
	t.Assert(len(containers), checker.Equals, 1)
	t.Assert(containers[0].Id, checker.Equals, "top")
	t.Assert(containers[0].Status, checker.Equals, "running")
	t.Assert(containers[0].BundlePath, check.Equals, filepath.Join(cs.cwd, GetBundle(bundleName).Path))
}

func (cs *ContainerdSuite) TestBusyboxTopExecTopKillInit(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	var (
		err   error
		initp *containerProcess
	)

	containerId := "top"
	initp, err = cs.StartContainer(containerId, bundleName)
	t.Assert(err, checker.Equals, nil)

	execId := "top1"
	_, err = cs.AddProcessToContainer(initp, execId, "/", []string{"PATH=/usr/bin"}, []string{"top"}, 0, 0)
	t.Assert(err, checker.Equals, nil)

	for idx, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerId,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "start-process",
			Id:     containerId,
			Status: 0,
			Pid:    execId,
		},
		{
			Type:   "exit",
			Id:     containerId,
			Status: 137,
			Pid:    execId,
		},
		{
			Type:   "exit",
			Id:     containerId,
			Status: 143,
			Pid:    "init",
		},
	} {
		ch := initp.GetEventsChannel()
		e := <-ch
		evt.Timestamp = e.Timestamp
		t.Assert(*e, checker.Equals, evt)
		if idx == 1 {
			// Process Started, kill it
			cs.SignalContainerProcess(containerId, "init", uint32(syscall.SIGTERM))
		}
	}
}
