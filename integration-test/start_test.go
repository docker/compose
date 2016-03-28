package main

import (
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (cs *ContainerdSuite) TestStartBusyboxLsSlash(t *check.C) {
	expectedOutput := `bin
dev
etc
home
lib
lib64
linuxrc
media
mnt
opt
proc
root
run
sbin
sys
tmp
usr
var
`
	if err := CreateBusyboxBundle("busybox-ls-slash", []string{"ls", "/"}); err != nil {
		t.Fatal(err)
	}

	c, err := cs.RunContainer("myls", "busybox-ls-slash")
	if err != nil {
		t.Fatal(err)
	}

	t.Assert(c.io.stdoutBuffer.String(), checker.Equals, expectedOutput)
}

func (cs *ContainerdSuite) TestStartBusyboxNoSuchFile(t *check.C) {
	expectedOutput := `oci runtime error: exec: \"NoSuchFile\": executable file not found in $PATH`

	if err := CreateBusyboxBundle("busybox-no-such-file", []string{"NoSuchFile"}); err != nil {
		t.Fatal(err)
	}

	_, err := cs.RunContainer("NoSuchFile", "busybox-no-such-file")
	t.Assert(err.Error(), checker.Contains, expectedOutput)
}

func (cs *ContainerdSuite) TestStartBusyboxTop(t *check.C) {
	if err := CreateBusyboxBundle("busybox-top", []string{"top"}); err != nil {
		t.Fatal(err)
	}

	_, err := cs.StartContainer("top", "busybox-top")
	t.Assert(err, checker.Equals, nil)
}

func (cs *ContainerdSuite) TestStartBusyboxLsEvents(t *check.C) {
	if err := CreateBusyboxBundle("busybox-ls", []string{"ls"}); err != nil {
		t.Fatal(err)
	}

	containerId := "ls-events"
	c, err := cs.StartContainer(containerId, "busybox-ls")
	if err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerId,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerId,
			Status: 0,
			Pid:    "init",
		},
	} {
		ch := c.GetEventsChannel()
		select {
		case e := <-ch:
			evt.Timestamp = e.Timestamp

			t.Assert(*e, checker.Equals, evt)
		case <-time.After(2 * time.Second):
			t.Fatal("Container took more than 2 seconds to terminate")
		}
	}
}

func (cs *ContainerdSuite) TestStartBusyboxSleep(t *check.C) {
	if err := CreateBusyboxBundle("busybox-sleep-5", []string{"sleep", "5"}); err != nil {
		t.Fatal(err)
	}

	ch := make(chan interface{})
	filter := func(e *types.Event) {
		if e.Type == "exit" && e.Pid == "init" {
			ch <- nil
		}
	}

	start := time.Now()
	_, err := cs.StartContainerWithEventFilter("sleep5", "busybox-sleep-5", filter)
	if err != nil {
		t.Fatal(err)
	}

	// We add a generous 20% marge of error
	select {
	case <-ch:
		t.Assert(uint64(time.Now().Sub(start)), checker.LessOrEqualThan, uint64(6*time.Second))
	case <-time.After(6 * time.Second):
		t.Fatal("Container took more than 6 seconds to exit")
	}
}
