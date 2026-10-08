//go:build !windows

/*
   Copyright 2026 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// containerExitDelay is how long after its start the test container exits
const containerExitDelay = 100 * time.Millisecond

// interactiveUpDaemon is the engine as an interactive `up` of a one-service
// project sees it: it records the order of the calls that matter, and
// reproduces the engine's event log (see fakeEventLog) so a container that
// exits as soon as it starts is observable whenever the monitor subscribes.
type interactiveUpDaemon struct {
	mu       sync.Mutex
	calls    []string
	created  bool
	eventLog *fakeEventLog
	// createErr, when set, fails ContainerCreate
	createErr error
	// slowSubscription delays the completion of the events subscription well
	// past the moment the container has started and exited
	slowSubscription bool
	started          chan struct{}
	startedOnce      sync.Once
}

func (d *interactiveUpDaemon) record(call string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, call)
}

func (d *interactiveUpDaemon) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

func (d *interactiveUpDaemon) indexOf(call string) int {
	for i, c := range d.recorded() {
		if c == call {
			return i
		}
	}
	return -1
}

func expectInteractiveUpDaemon(t *testing.T, apiClient *mocks.MockAPIClient) *interactiveUpDaemon {
	t.Helper()
	d := &interactiveUpDaemon{eventLog: &fakeEventLog{}, started: make(chan struct{})}
	labels := map[string]string{
		api.ProjectLabel:         "test",
		api.ServiceLabel:         "web",
		api.OneoffLabel:          "False",
		api.ContainerNumberLabel: "1",
		api.ConfigHashLabel:      "hash",
	}

	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine", gomock.Any()).AnyTimes().
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:alpine"}}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.VolumeListResult{}, nil)

	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			if !d.created {
				return client.ContainerListResult{}, nil
			}
			return client.ContainerListResult{Items: []container.Summary{{
				ID: "c1", Names: []string{"/test-web-1"}, Labels: labels, State: container.StateCreated,
			}}}, nil
		})
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			d.record("create")
			if d.createErr != nil {
				return client.ContainerCreateResult{}, d.createErr
			}
			d.mu.Lock()
			d.created = true
			d.mu.Unlock()
			return client.ContainerCreateResult{ID: "c1"}, nil
		})
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:              "c1",
			Name:            "/test-web-1",
			Config:          &container.Config{Labels: labels},
			State:           &container.State{Status: container.StateExited},
			NetworkSettings: &container.NetworkSettings{},
		}}, nil)

	// no attach endpoint: the log stream falls back to the logs API, which
	// the monitor-less assertions below don't read
	apiClient.EXPECT().ContainerAttach(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, string, client.ContainerAttachOptions) (client.ContainerAttachResult, error) {
			d.record("attach")
			return client.ContainerAttachResult{}, errors.New("attach not supported")
		})
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		Return(io.NopCloser(strings.NewReader("")), nil)

	// the monitor reads the daemon's time to know from when to ask for events
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
			return d.eventLog.info()
		})
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, opts client.EventsListOptions) client.EventsResult {
			d.record("events")
			if d.slowSubscription {
				select {
				case <-d.started:
					time.Sleep(containerExitDelay + 200*time.Millisecond)
				case <-time.After(5 * time.Second):
				}
			}
			return d.eventLog.subscribe(t, opts)
		})

	// a teardown (the test giving up on a hung up) must not trip the mock
	apiClient.EXPECT().ContainerStop(gomock.Any(), "c1", gomock.Any()).AnyTimes().Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerKill(gomock.Any(), "c1", gomock.Any()).AnyTimes().Return(client.ContainerKillResult{}, nil)

	// the container exits right after it starts
	apiClient.EXPECT().ContainerStart(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			d.record("start")
			defer d.startedOnce.Do(func() { close(d.started) })
			d.eventLog.emit(containerMessage(events.ActionStart, "c1", "test-web-1", "web", labels))
			// The engine reports the exit some time after the start call, not
			// within it: long enough here for the monitor, launched just
			// before the Start phase, to have taken its timestamp.
			timer := time.AfterFunc(containerExitDelay, func() {
				d.eventLog.emit(containerMessage(events.ActionDie, "c1", "test-web-1", "web", map[string]string{"exitCode": "0"}))
			})
			t.Cleanup(func() { timer.Stop() })
			return client.ContainerStartResult{}, nil
		})
	return d
}

// progressRecorder records the lifetime of the progress scopes ("up") in the
// daemon's call log, to order them against the engine calls.
type progressRecorder struct{ d *interactiveUpDaemon }

func (p progressRecorder) Start(_ context.Context, operation string) {
	p.d.record("progress-start:" + operation)
}

func (p progressRecorder) Done(operation string, _ bool) { p.d.record("progress-done:" + operation) }
func (p progressRecorder) On(...api.Resource)            {}

// interactiveUpHarness is a compose service wired to the interactiveUpDaemon.
type interactiveUpHarness struct {
	svc    *composeService
	daemon *interactiveUpDaemon
	out    *bytes.Buffer
}

func newInteractiveUpHarness(t *testing.T) *interactiveUpHarness {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	out := &bytes.Buffer{}
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	cli.EXPECT().Out().Return(streams.NewOut(out)).AnyTimes()
	cli.EXPECT().Err().Return(streams.NewOut(io.Discard)).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("unix:///var/run/docker.sock").AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

	daemon := expectInteractiveUpDaemon(t, apiClient)
	svc, err := NewComposeService(cli, WithEventProcessor(progressRecorder{daemon}))
	assert.NilError(t, err)
	return &interactiveUpHarness{svc: svc.(*composeService), daemon: daemon, out: out}
}

func interactiveUpProject() *types.Project {
	return &types.Project{
		Name:     "test",
		Services: types.Services{"web": {Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}},
		Networks: types.Networks{},
		Volumes:  types.Volumes{},
	}
}

func interactiveUpOptions() api.UpOptions {
	return api.UpOptions{Start: api.StartOptions{Attach: &testLogConsumer{}, AttachTo: []string{"web"}}}
}

// up runs Up, failing the test instead of hanging when it doesn't return.
func (h *interactiveUpHarness) up(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.svc.Up(ctx, interactiveUpProject(), interactiveUpOptions()) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		t.Fatalf("up did not return; calls so far: %v", h.daemon.recorded())
		return nil
	}
}

// TestUpInteractivePhaseOrder pins the sequence of an interactive up: the
// Create phase runs under the "up" progress scope and completes, the session
// then attaches to the created container, and only then is the Start phase
// dispatched, outside that scope (whose refresh would repaint over the logs).
func TestUpInteractivePhaseOrder(t *testing.T) {
	h := newInteractiveUpHarness(t)

	assert.NilError(t, h.up(t))

	d := h.daemon
	create, done, attach, start := d.indexOf("create"), d.indexOf("progress-done:up"), d.indexOf("attach"), d.indexOf("start")
	assert.Assert(t, create >= 0 && done >= 0 && attach >= 0 && start >= 0, "calls: %v", d.recorded())
	assert.Assert(t, create < done, "the container is created within the up progress scope: %v", d.recorded())
	assert.Assert(t, done < attach, "the session attaches once the up progress scope is over: %v", d.recorded())
	assert.Assert(t, attach < start, "no container starts before the session is attached: %v", d.recorded())
}

// TestUpInteractiveSeesContainerExitingBeforeSubscription is the monitor's
// startup race seen from interactive up: the Start phase runs right after the
// monitor is launched, so the container may start and exit before the event
// subscription is effective (modeled here as the subscription only completing
// once the container has started). The exit must still be observed, or up
// would wait on the container forever. The replay starts at the daemon's time,
// read before the container listing: the daemon's clock is set here well apart
// from the client's, as it is for a remote daemon, so that the events it
// stamps are only replayed when asked for from its own time.
func TestUpInteractiveSeesContainerExitingBeforeSubscription(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.slowSubscription = true
	h.daemon.eventLog.skew = -time.Hour

	assert.NilError(t, h.up(t))

	log := h.daemon.eventLog
	assert.Equal(t, len(log.since), 1)
	assert.Assert(t, log.since[0] != "", "the events subscription must replay what preceded it")
	reported, err := time.Parse(time.RFC3339Nano, log.reportedTime)
	assert.NilError(t, err)
	assert.Assert(t, parseEventsSince(t, log.since[0]).Equal(reported),
		"since %s is not the daemon time %s", log.since[0], log.reportedTime)
}

// TestUpInteractiveFailsWithoutDaemonTime: the monitor replays events from the
// daemon's time, and a daemon whose time can't be read fails the up, with that
// explicit error, before the Start phase: no container is started, and no
// event subscription was ever opened.
func TestUpInteractiveFailsWithoutDaemonTime(t *testing.T) {
	unreachable := errors.New("daemon unreachable")
	tests := []struct {
		name  string
		setup func(*fakeEventLog)
		// wantIs is the error the failure wraps, if any
		wantIs error
	}{
		{name: "info fails", setup: func(log *fakeEventLog) { log.infoErr = unreachable }, wantIs: unreachable},
		{name: "system time is empty", setup: func(log *fakeEventLog) { log.setSystemTime("") }},
		{name: "system time is not a time", setup: func(log *fakeEventLog) { log.setSystemTime("yesterday-ish") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newInteractiveUpHarness(t)
			tt.setup(h.daemon.eventLog)

			err := h.up(t)

			assert.ErrorContains(t, err, "reading the daemon time to subscribe to container events")
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
			calls := h.daemon.recorded()
			assert.Assert(t, h.daemon.indexOf("create") >= 0, "the Create phase ran: %v", calls)
			assert.Equal(t, h.daemon.indexOf("start"), -1, "no container is started: %v", calls)
			assert.Equal(t, h.daemon.indexOf("events"), -1, "no event subscription is opened: %v", calls)
		})
	}
}

// TestUpInteractiveFailedCreateStopsThere: a Create phase that fails reaches
// neither the attach session nor the Start phase.
func TestUpInteractiveFailedCreateStopsThere(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.createErr = errors.New("no space left on device")

	err := h.up(t)

	assert.ErrorContains(t, err, "no space left on device")
	assert.DeepEqual(t, h.daemon.recorded(), []string{"progress-start:up", "create", "progress-done:up"})
}

// TestUpInteractiveDryRunOnlyCreates: a dry-run interactive up plans the
// Create phase and never gets to the session or the Start phase.
func TestUpInteractiveDryRunOnlyCreates(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.svc.dryRun = true

	assert.NilError(t, h.up(t))

	assert.DeepEqual(t, h.daemon.recorded(), []string{"progress-start:up", "create", "progress-done:up"})
	assert.Assert(t, strings.Contains(h.out.String(), "interactive run is not supported in dry-run mode"), "output: %q", h.out.String())
}
