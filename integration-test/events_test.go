package main

import (
	"fmt"
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (cs *ContainerdSuite) TestEventsId(t *check.C) {
	if err := CreateBusyboxBundle("busybox-ls", []string{"ls"}); err != nil {
		t.Fatal(err)
	}

	from := time.Now()

	for i := 0; i < 10; i++ {
		_, err := cs.RunContainer(fmt.Sprintf("ls-%d", i), "busybox-ls")
		if err != nil {
			t.Fatal(err)
		}
	}

	containerID := "ls-4"

	events, err := cs.Events(from, true, containerID)
	if err != nil {
		t.Fatal(err)
	}

	evs := []*types.Event{}
	for {
		e, err := events.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatal(err)
		}
		evs = append(evs, e)
	}

	t.Assert(len(evs), checker.Equals, 2)
	for idx, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerID,
			Status: 0,
			Pid:    "init",
		},
	} {
		evt.Timestamp = evs[idx].Timestamp
		t.Assert(*evs[idx], checker.Equals, evt)
	}
}
