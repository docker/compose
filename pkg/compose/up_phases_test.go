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
	"sync/atomic"
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
	// onCreate, onAttach and onInfo, when set, run when the container is
	// created, once the session attached to it, and once it read the daemon's
	// time
	onCreate func()
	onAttach func()
	onInfo   func()
	// startWaitsForEvents holds a container start until the events
	// subscription is opened, giving up (and flagging it) after a while
	startWaitsForEvents   bool
	eventsSeen            chan struct{}
	eventsOnce            sync.Once
	startedWithoutMonitor atomic.Bool
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
	d := &interactiveUpDaemon{eventLog: &fakeEventLog{}, started: make(chan struct{}), eventsSeen: make(chan struct{})}
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
		DoAndReturn(func(ctx context.Context, _ client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			d.record("create")
			if d.onCreate != nil {
				d.onCreate()
			}
			// like the engine's client: an interrupted call reports it
			if err := ctx.Err(); err != nil {
				return client.ContainerCreateResult{}, err
			}
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
			if d.onAttach != nil {
				d.onAttach()
			}
			return client.ContainerAttachResult{}, errors.New("attach not supported")
		})
	apiClient.EXPECT().ContainerLogs(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		Return(io.NopCloser(strings.NewReader("")), nil)

	// the monitor reads the daemon's time to know from when to ask for events
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(ctx context.Context, _ client.InfoOptions) (client.SystemInfoResult, error) {
			if err := ctx.Err(); err != nil {
				return client.SystemInfoResult{}, err
			}
			res, err := d.eventLog.info()
			if d.onInfo != nil {
				d.onInfo()
			}
			return res, err
		})
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ context.Context, opts client.EventsListOptions) client.EventsResult {
			d.record("events")
			d.eventsOnce.Do(func() { close(d.eventsSeen) })
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
			if d.startWaitsForEvents {
				select {
				case <-d.eventsSeen:
				case <-time.After(5 * time.Second):
					d.startedWithoutMonitor.Store(true)
				}
			}
			d.record("start")
			defer d.startedOnce.Do(func() { close(d.started) })
			d.eventLog.emit(containerMessage(events.ActionStart, "c1", "test-web-1", "web", labels))
			// The engine reports the exit some time after the start call, not
			// within it.
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
	return h.upWith(t, ctx, cancel)
}

// upWith is up on a context the test owns, to cancel it at a given point.
func (h *interactiveUpHarness) upWith(t *testing.T, ctx context.Context, cancel context.CancelFunc) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.svc.Up(ctx, interactiveUpProject(), interactiveUpOptions()) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		cancel()
		select {
		case <-done: //nolint:errcheck // only waiting for up to return, the test fails right below
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
	scope, create, done := d.indexOf("progress-start:up"), d.indexOf("create"), d.indexOf("progress-done:up")
	attach, start := d.indexOf("attach"), d.indexOf("start")
	assert.Assert(t, scope >= 0 && create >= 0 && done >= 0 && attach >= 0 && start >= 0, "calls: %v", d.recorded())
	assert.Assert(t, scope < create && create < done, "the container is created within the up progress scope: %v", d.recorded())
	assert.Assert(t, done < attach, "the session attaches once the up progress scope is over: %v", d.recorded())
	assert.Assert(t, attach < start, "no container starts before the session is attached: %v", d.recorded())
}

// TestUpInteractiveSeesContainerExitingBeforeSubscription is the monitor's
// startup race seen from interactive up: the Start phase runs right after the
// monitor is launched, so the container may start and exit before the event
// subscription is effective (modeled here as the subscription only completing
// once the container has started). The exit must still be observed, or up
// would wait on the container forever. The replay starts at the daemon's time,
// which the session reads before the Start phase: the daemon's clock is set
// here well apart from the client's, as it is for a remote daemon, so that the
// events it stamps are only replayed when asked for from its own time.
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
// daemon's time, and a daemon that answers with a time that can't be used fails
// the up, with that explicit error, before the Start phase: no container is
// started, and no event subscription was ever opened.
func TestUpInteractiveFailsWithoutDaemonTime(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeEventLog)
	}{
		{name: "system time is empty", setup: func(log *fakeEventLog) { log.setSystemTime("") }},
		{name: "system time is not a time", setup: func(log *fakeEventLog) { log.setSystemTime("yesterday-ish") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newInteractiveUpHarness(t)
			tt.setup(h.daemon.eventLog)

			err := h.up(t)

			assert.ErrorContains(t, err, "reading the daemon time to subscribe to container events")
			calls := h.daemon.recorded()
			assert.Assert(t, h.daemon.indexOf("create") >= 0, "the Create phase ran: %v", calls)
			assert.Equal(t, h.daemon.indexOf("start"), -1, "no container is started: %v", calls)
			assert.Equal(t, h.daemon.indexOf("events"), -1, "no event subscription is opened: %v", calls)
		})
	}
}

// TestUpInteractiveWithoutInfoStillStarts: a daemon that can't be asked for its
// time (a read-only socket proxy that doesn't expose the info endpoint) doesn't
// fail the up, which worked without it: the containers start, and the monitor
// subscribes to live events, with no starting point.
func TestUpInteractiveWithoutInfoStillStarts(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.eventLog.infoErr = errors.New("403 Forbidden")

	assert.NilError(t, h.up(t))

	assert.Assert(t, h.daemon.indexOf("start") >= 0, "the Start phase ran: %v", h.daemon.recorded())
	assert.DeepEqual(t, h.daemon.eventLog.since, []string{""})
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

// TestUpInteractiveDryRunOnlyCreates: a dry-run interactive up stops after the
// Create phase: no session, no event subscription, no start. Only the service's
// dry-run flag is set here; the engine calls a real dry run swaps for the dry-run
// client are not what this pins, but the gate of Up.
func TestUpInteractiveDryRunOnlyCreates(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.svc.dryRun = true

	assert.NilError(t, h.up(t))

	for _, call := range []string{"attach", "events", "start"} {
		assert.Equal(t, h.daemon.indexOf(call), -1, "%s must not be reached in a dry run: %v", call, h.daemon.recorded())
	}
	assert.Assert(t, strings.Contains(h.out.String(), "interactive run is not supported in dry-run mode"), "output: %q", h.out.String())
}

// TestUpInteractiveStartPhaseSurvivesCancellation: the Start phase runs on a
// context that outlives the caller's, since an interruption is the session's to
// handle once it is set up (it stops what the application started). Canceled
// right after the session read the daemon's time, which is its last step before
// the Start phase, up still starts the container.
func TestUpInteractiveStartPhaseSurvivesCancellation(t *testing.T) {
	h := newInteractiveUpHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.daemon.onInfo = cancel

	_ = h.upWith(t, ctx, cancel)

	assert.Assert(t, h.daemon.indexOf("start") >= 0, "the Start phase ran to the container start: %v", h.daemon.recorded())
}

// TestUpInteractiveInterruptedBeforeTheStartPhase: an interruption met while
// the session reads the daemon's time is reported as such, wrapping the
// context's error, which the CLI maps to its interruption exit code, and no
// container is started.
func TestUpInteractiveInterruptedBeforeTheStartPhase(t *testing.T) {
	h := newInteractiveUpHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.daemon.onAttach = cancel

	err := h.upWith(t, ctx, cancel)

	assert.Assert(t, errors.Is(err, context.Canceled), "err: %v", err)
	assert.Equal(t, h.daemon.indexOf("start"), -1, "no container is started: %v", h.daemon.recorded())
}

// TestUpInteractiveMonitorRunsBeforeTheStartPhase: the monitor follows the
// application from the moment the Start phase begins, which can last as long as
// its dependency waits: the cascade, the late-starter stop and the log
// following are dead until it runs. The fake engine holds a container start
// until the events subscription is opened, which only the monitor does: a
// Create-phase use of the events API would make this test vacuous.
func TestUpInteractiveMonitorRunsBeforeTheStartPhase(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.startWaitsForEvents = true

	assert.NilError(t, h.up(t))

	assert.Assert(t, !h.daemon.startedWithoutMonitor.Load(), "a container start came before the monitor was subscribing: %v", h.daemon.recorded())
}

// TestUpInteractiveInterruptedDuringTheCreatePhase: an interruption met by the
// Create phase ends the up with it: the phase runs on the caller's context,
// whose cancellation the engine reports, and the session is never set up.
func TestUpInteractiveInterruptedDuringTheCreatePhase(t *testing.T) {
	h := newInteractiveUpHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.daemon.onCreate = cancel

	err := h.upWith(t, ctx, cancel)

	assert.Assert(t, errors.Is(err, context.Canceled), "err: %v", err)
	for _, call := range []string{"attach", "events", "start"} {
		assert.Equal(t, h.daemon.indexOf(call), -1, "%s must not be reached: %v", call, h.daemon.recorded())
	}
}

// TestUpInteractiveCreatePhaseCarriesThePlanProject: the Start phase, the
// session, the watcher and the teardown work on the plan's own project, which
// carries what preparation resolved -- here use_api_socket's #apisocket config,
// which the caller's project never gets.
func TestUpInteractiveCreatePhaseCarriesThePlanProject(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.svc.contextInfo = linuxContextInfo{}
	project := interactiveUpProject()
	project.Configs = types.Configs{}
	web := project.Services["web"]
	web.UseAPISocket = true
	web.Environment = types.MappingWithEquals{}
	project.Services["web"] = web

	created, err := h.svc.upCreatePhase(t.Context(), project, interactiveUpOptions())

	assert.NilError(t, err)
	_, plan := created.project.Configs["#apisocket"]
	assert.Assert(t, plan, "the plan's project carries the use_api_socket config")
	_, caller := project.Configs["#apisocket"]
	assert.Assert(t, !caller, "the caller's project is not transformed")
}
