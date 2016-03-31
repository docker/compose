package main

import (
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
