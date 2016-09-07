package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
	ocs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc"
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
	expectedOutput := `oci runtime error: exec: "NoSuchFile": executable file not found in $PATH`

	if err := CreateBusyboxBundle("busybox-no-such-file", []string{"NoSuchFile"}); err != nil {
		t.Fatal(err)
	}

	_, err := cs.RunContainer("NoSuchFile", "busybox-no-such-file")
	t.Assert(grpc.ErrorDesc(err), checker.Contains, expectedOutput)
}

func (cs *ContainerdSuite) TestStartBusyboxTop(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	containerID := "start-busybox-top"
	_, err := cs.StartContainer(containerID, bundleName)
	t.Assert(err, checker.Equals, nil)

	containers, err := cs.ListRunningContainers()
	if err != nil {
		t.Fatal(err)
	}
	t.Assert(len(containers), checker.Equals, 1)
	t.Assert(containers[0].Id, checker.Equals, containerID)
	t.Assert(containers[0].Status, checker.Equals, "running")
	t.Assert(containers[0].BundlePath, check.Equals, filepath.Join(cs.cwd, GetBundle(bundleName).Path))
}

func (cs *ContainerdSuite) TestStartBusyboxLsEvents(t *check.C) {
	if err := CreateBusyboxBundle("busybox-ls", []string{"ls"}); err != nil {
		t.Fatal(err)
	}

	containerID := "ls-events"
	c, err := cs.StartContainer(containerID, "busybox-ls")
	if err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
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

func (cs *ContainerdSuite) TestStartBusyboxTopKill(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	containerID := "top-kill"
	c, err := cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}

	<-time.After(1 * time.Second)

	err = cs.KillContainer(containerID)
	if err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerID,
			Status: 128 + uint32(syscall.SIGKILL),
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

func (cs *ContainerdSuite) TestStartBusyboxTopSignalSigterm(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	containerID := "top-sigterm"
	c, err := cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}

	<-time.After(1 * time.Second)

	err = cs.SignalContainer(containerID, uint32(syscall.SIGTERM))
	if err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerID,
			Status: 128 + uint32(syscall.SIGTERM),
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

func (cs *ContainerdSuite) TestStartBusyboxTrapUSR1(t *check.C) {
	if err := CreateBusyboxBundle("busybox-trap-usr1", []string{"sh", "-c", "trap 'echo -n booh!' SIGUSR1 ; sleep 60  &  wait"}); err != nil {
		t.Fatal(err)
	}

	containerID := "trap-usr1"
	c, err := cs.StartContainer(containerID, "busybox-trap-usr1")
	if err != nil {
		t.Fatal(err)
	}

	<-time.After(1 * time.Second)

	if err := cs.SignalContainer(containerID, uint32(syscall.SIGUSR1)); err != nil {
		t.Fatal(err)
	}

	for {
		e := c.GetNextEvent()
		if e.Type == "exit" && e.Pid == "init" {
			break
		}
	}

	t.Assert(c.io.stdoutBuffer.String(), checker.Equals, "booh!")
}

func (cs *ContainerdSuite) TestStartBusyboxTopPauseResume(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	containerID := "top-pause-resume"
	c, err := cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}

	if err := cs.PauseContainer(containerID); err != nil {
		t.Fatal(err)
	}

	if err := cs.ResumeContainer(containerID); err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "pause",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "resume",
			Id:     containerID,
			Status: 0,
			Pid:    "",
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

	// check that status is running
	containers, err := cs.ListRunningContainers()
	if err != nil {
		t.Fatal(err)
	}
	t.Assert(len(containers), checker.Equals, 1)
	t.Assert(containers[0].Id, checker.Equals, containerID)
	t.Assert(containers[0].Status, checker.Equals, "running")
}

func (cs *ContainerdSuite) TestOOM(t *check.C) {
	bundleName := "busybox-sh-512k-memlimit"
	if err := CreateBundleWithFilter("busybox", bundleName, []string{"sh", "-c", "x=oom-party-time; while true; do x=$x$x$x$x$x$x$x$x$x$x; done"}, func(spec *ocs.Spec) {
		// Limit to 512k for quick oom
		var limit uint64 = 8 * 1024 * 1024
		spec.Linux.Resources.Memory = &ocs.Memory{
			Limit: &limit,
		}
		if swapEnabled() {
			spec.Linux.Resources.Memory.Swap = &limit
		}
	}); err != nil {
		t.Fatal(err)
	}

	containerID := "sh-oom"
	c, err := cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}

	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "oom",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerID,
			Status: 137,
			Pid:    "init",
		},
	} {
		ch := c.GetEventsChannel()
		select {
		case e := <-ch:
			evt.Timestamp = e.Timestamp
			t.Assert(*e, checker.Equals, evt)
		case <-time.After(60 * time.Second):
			t.Fatalf("Container took more than 60 seconds to %s", evt.Type)
		}
	}
}

func (cs *ContainerdSuite) TestRestart(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}

	totalCtr := 10

	for i := 0; i < totalCtr; i++ {
		containerID := fmt.Sprintf("top%d", i)
		c, err := cs.StartContainer(containerID, bundleName)
		if err != nil {
			t.Fatal(err)
		}

		e := c.GetNextEvent()

		t.Assert(*e, checker.Equals, types.Event{
			Type:      "start-container",
			Id:        containerID,
			Status:    0,
			Pid:       "",
			Timestamp: e.Timestamp,
		})
	}

	// restart daemon gracefully (SIGINT)
	cs.RestartDaemon(false)

	// check that status is running
	containers, err := cs.ListRunningContainers()
	if err != nil {
		t.Fatal(err)
	}
	sortContainers(containers)
	t.Assert(len(containers), checker.Equals, totalCtr)
	for i := 0; i < totalCtr; i++ {
		t.Assert(containers[i].Id, checker.Equals, fmt.Sprintf("top%d", i))
		t.Assert(containers[i].Status, checker.Equals, "running")
	}

	// Now kill daemon (SIGKILL)
	cs.StopDaemon(true)

	// Sleep a second to allow thevent e timestamp to change since
	// it's second based
	<-time.After(3 * time.Second)

	// Kill a couple of containers
	killedCtr := map[int]bool{4: true, 2: true}

	var f func(*types.Event)
	deathChans := make([]chan error, len(killedCtr))
	deathChansIdx := 0

	for i := range killedCtr {
		ch := make(chan error, 1)
		deathChans[deathChansIdx] = ch
		deathChansIdx++
		syscall.Kill(int(containers[i].Pids[0]), syscall.SIGKILL)

		// Filter to be notified of their death
		containerID := fmt.Sprintf("top%d", i)
		f = func(event *types.Event) {
			expectedEvent := types.Event{
				Type:   "exit",
				Id:     containerID,
				Status: 137,
				Pid:    "init",
			}
			expectedEvent.Timestamp = event.Timestamp
			if ok := t.Check(*event, checker.Equals, expectedEvent); !ok {
				ch <- fmt.Errorf("Unexpected event: %#v", *event)
			} else {
				ch <- nil
			}
		}
		cs.SetContainerEventFilter(containerID, f)
	}

	cs.RestartDaemon(true)

	// Ensure we got our events
	for i := range deathChans {
		done := false
		for done == false {
			select {
			case err := <-deathChans[i]:
				t.Assert(err, checker.Equals, nil)
				done = true
			case <-time.After(3 * time.Second):
				t.Fatal("Exit event for container not received after 3 seconds")
			}
		}
	}

	// check that status is running
	containers, err = cs.ListRunningContainers()
	if err != nil {
		t.Fatal(err)
	}
	sortContainers(containers)
	t.Assert(len(containers), checker.Equals, totalCtr-len(killedCtr))
	idShift := 0
	for i := 0; i < totalCtr-len(killedCtr); i++ {
		if _, ok := killedCtr[i+idShift]; ok {
			idShift++
		}
		t.Assert(containers[i].Id, checker.Equals, fmt.Sprintf("top%d", i+idShift))
		t.Assert(containers[i].Status, checker.Equals, "running")
	}
}

func swapEnabled() bool {
	_, err := os.Stat("/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes")
	return err == nil
}

func (cs *ContainerdSuite) TestSigkillShimReuseName(t *check.C) {
	bundleName := "busybox-top"
	if err := CreateBusyboxBundle(bundleName, []string{"top"}); err != nil {
		t.Fatal(err)
	}
	containerID := "top"
	c, err := cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}

	// Sigkill the shim
	exec.Command("pkill", "-9", "containerd-shim").Run()

	// Wait for it to be reaped
	for _, evt := range []types.Event{
		{
			Type:   "start-container",
			Id:     containerID,
			Status: 0,
			Pid:    "",
		},
		{
			Type:   "exit",
			Id:     containerID,
			Status: 128 + 9,
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

	// Start a new continer with the same name
	c, err = cs.StartContainer(containerID, bundleName)
	if err != nil {
		t.Fatal(err)
	}
}
