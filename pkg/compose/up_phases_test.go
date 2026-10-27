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
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	dockercli "github.com/docker/cli/cli"
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
	// neverExits keeps the container running once started: no die event
	neverExits bool
	// startGate, when set, holds ContainerStart (after recording
	// "start-requested") until it is closed, and then fails it with startErr
	// if that is set
	startGate chan struct{}
	startErr  error
	// signals closes, the first time a call is recorded, the channel
	// callSignal hands out for it
	signals map[string]chan struct{}
	// listErr, when set, fails the container listings made once the container
	// is created
	listErr error
	// inspectState is the state the engine reports for the container; it
	// reports it exited when unset
	inspectState *container.State
	// stopEndsStartedContainer makes the engine report the container dead when
	// it is stopped once started; stopEndsContainer, whatever its state
	stopEndsStartedContainer bool
	stopEndsContainer        bool
	// exitCode is the exit code of the container when it exits, "0" if unset
	exitCode string
}

// callSignal returns a channel closed once call has been recorded, to wait for
// an engine call without polling.
func (d *interactiveUpDaemon) callSignal(call string) <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.signalLocked(call)
}

func (d *interactiveUpDaemon) signalLocked(call string) chan struct{} {
	if d.signals == nil {
		d.signals = map[string]chan struct{}{}
	}
	ch, ok := d.signals[call]
	if !ok {
		ch = make(chan struct{})
		d.signals[call] = ch
	}
	return ch
}

func (d *interactiveUpDaemon) record(call string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, call)
	ch := d.signalLocked(call)
	select {
	case <-ch:
	default:
		close(ch)
	}
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
			if d.listErr != nil {
				return client.ContainerListResult{}, d.listErr
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
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			state := d.inspectState
			if state == nil {
				state = &container.State{Status: container.StateExited}
			}
			return client.ContainerInspectResult{Container: container.InspectResponse{
				ID:              "c1",
				Name:            "/test-web-1",
				Config:          &container.Config{Labels: labels},
				State:           state,
				NetworkSettings: &container.NetworkSettings{},
			}}, nil
		})

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
	apiClient.EXPECT().ContainerStop(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error) {
			d.record("stop")
			if d.stopEndsContainer {
				d.eventLog.emit(containerMessage(events.ActionDie, "c1", "test-web-1", "web", map[string]string{"exitCode": "143"}))
			} else if d.stopEndsStartedContainer {
				select {
				case <-d.started:
					d.eventLog.emit(containerMessage(events.ActionDie, "c1", "test-web-1", "web", map[string]string{"exitCode": "143"}))
				default:
				}
			}
			return client.ContainerStopResult{}, nil
		})
	apiClient.EXPECT().ContainerKill(gomock.Any(), "c1", gomock.Any()).AnyTimes().Return(client.ContainerKillResult{}, nil)

	// the container exits right after it starts
	apiClient.EXPECT().ContainerStart(gomock.Any(), "c1", gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			return client.ContainerStartResult{}, d.containerStart(t, labels)
		})
	return d
}

// containerStart is the engine starting the container: once the events
// subscription is open, if the test wants it so, and the start gate lifted,
// when it is set; it then emits the start event and, unless the container is
// to run for good, schedules its exit.
func (d *interactiveUpDaemon) containerStart(t *testing.T, labels map[string]string) error {
	if d.startWaitsForEvents {
		select {
		case <-d.eventsSeen:
		case <-time.After(5 * time.Second):
			d.startedWithoutMonitor.Store(true)
		}
	}
	if d.startGate != nil {
		d.record("start-requested")
		<-d.startGate
		if d.startErr != nil {
			return d.startErr
		}
	}
	d.record("start")
	// before the start event: whoever reacts to it (the session stopping a
	// late starter) finds the container started
	d.startedOnce.Do(func() { close(d.started) })
	d.eventLog.emit(containerMessage(events.ActionStart, "c1", "test-web-1", "web", labels))
	if d.neverExits {
		return nil
	}
	// The engine reports the exit some time after the start call, not
	// within it.
	exitCode := d.exitCode
	if exitCode == "" {
		exitCode = "0"
	}
	timer := time.AfterFunc(containerExitDelay, func() {
		d.eventLog.emit(containerMessage(events.ActionDie, "c1", "test-web-1", "web", map[string]string{"exitCode": exitCode}))
	})
	t.Cleanup(func() { timer.Stop() })
	return nil
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
	// api is the engine's client, for the expectations a test adds to the
	// daemon's
	api *mocks.MockAPIClient
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
	return &interactiveUpHarness{svc: svc.(*composeService), daemon: daemon, out: out, api: apiClient}
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
	return h.upWithOptions(t, ctx, cancel, interactiveUpOptions())
}

// upWithOptions is upWith for given options.
func (h *interactiveUpHarness) upWithOptions(t *testing.T, ctx context.Context, cancel context.CancelFunc, options api.UpOptions) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- h.svc.Up(ctx, interactiveUpProject(), options) }()
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

// TestUpInteractiveFailsOnTheStartPhaseErrorOfAnUninterruptedUp pins that
// nothing excuses a Start phase failing in an up nobody interrupted: it is the
// up's error (see TestUpInteractiveIgnoresTheStartPhaseErrorOfAnInterruptedUp
// for the case it is not).
func TestUpInteractiveFailsOnTheStartPhaseErrorOfAnUninterruptedUp(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.startGate = make(chan struct{})
	h.daemon.startErr = errors.New("cannot start the container")
	close(h.daemon.startGate)

	err := h.up(t)
	assert.ErrorContains(t, err, "cannot start the container")
}

// TestUpInteractiveWatchesTheServicesItTargets pins which containers the
// session follows, and so ends with: those of the services the up targets
// (Start.Services) when it has some, those it attaches to otherwise. The
// container here never exits, and what is watched is a service with no
// container: the session ends as soon as the Start phase is over. Were the
// monitor watching the container too, the up would hang with it.
func TestUpInteractiveWatchesTheServicesItTargets(t *testing.T) {
	tests := []struct {
		name     string
		services []string
		attachTo []string
	}{
		{name: "the targeted services, not the attached ones", services: []string{"other"}, attachTo: []string{"web"}},
		{name: "the attached services when none is targeted", attachTo: []string{"other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newInteractiveUpHarness(t)
			h.daemon.neverExits = true
			options := interactiveUpOptions()
			options.Start.Services = tt.services
			options.Start.AttachTo = tt.attachTo
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			assert.NilError(t, h.upWithOptions(t, ctx, cancel, options))
		})
	}
}

// TestUpReportsTheContainersItLeavesRunning pins that an up reports as Running
// the containers its plan won't touch, so that the progress display accounts
// for the whole project: the plan holds nothing for them. It is the
// preparation of the plan, which a detached and an interactive up share, that
// does it.
func TestUpReportsTheContainersItLeavesRunning(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)
	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine", gomock.Any()).AnyTimes().
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:alpine"}}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.VolumeListResult{}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{Items: []container.Summary{{
		ID:    "c1",
		Names: []string{"/test-web-1"},
		State: container.StateRunning,
		Labels: map[string]string{
			api.ProjectLabel:         "test",
			api.ServiceLabel:         "web",
			api.OneoffLabel:          "False",
			api.ContainerNumberLabel: "1",
			api.ConfigHashLabel:      "whatever",
		},
	}}}, nil).AnyTimes()

	// never recreate: the running container stays whatever its configuration
	options := api.UpOptions{Create: api.CreateOptions{Recreate: api.RecreateNever}}
	_, _, plan, err := svc.prepareUp(t.Context(), interactiveUpProject(), options)

	assert.NilError(t, err)
	assert.Equal(t, len(plan.Nodes), 0, "the plan leaves the running container alone:\n%s", plan)
	assert.DeepEqual(t, recorder.byID["Container test-web-1"], []string{api.StatusRunning})
}

// createdUpOf builds by hand what an interactive up carries from its Create
// phase to its Start phase, for a plan the test writes itself: a Create phase
// with nothing to do, and the Start phase of plan. It also returns the
// executor, to give it what the daemon's listing would have.
func (h *interactiveUpHarness) createdUpOf(t *testing.T, project *types.Project, plan *Plan) (*createdUp, *planExecutor) {
	t.Helper()
	exec := h.svc.newPlanExecutor(project, emptyObservedState(project.Name))
	run, err := exec.begin(plan)
	assert.NilError(t, err)
	assert.NilError(t, run.runCreate(t.Context()))
	return &createdUp{project: project, run: run}, exec
}

// runInteractiveUp runs the session of created, failing the test instead of
// hanging when it doesn't return.
func (h *interactiveUpHarness) runInteractiveUp(t *testing.T, created *createdUp) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.svc.runInteractiveUp(ctx, created, interactiveUpOptions()) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatalf("the session did not return; calls so far: %v", h.daemon.recorded())
		return nil
	}
}

func webContainerSummary() container.Summary {
	return container.Summary{ID: "c1", Names: []string{"/test-web-1"}, Labels: map[string]string{api.ServiceLabel: "web"}}
}

// TestUpInteractiveShowsTheHookLogsInTheSession pins that what the post_start
// hook of a service prints during an interactive up's Start phase reaches the
// session's log consumer, as the output of a container does: the session's
// printer is the listener the Start phase is given.
func TestUpInteractiveShowsTheHookLogsInTheSession(t *testing.T) {
	h := newInteractiveUpHarness(t)
	project := interactiveUpProject()
	web := project.Services["web"]
	web.PostStart = []types.ServiceHook{{Command: types.ShellCommand{"/notify.sh"}}}
	project.Services["web"] = web
	ctr := webContainerSummary()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close() //nolint:errcheck
		clientConn.Close() //nolint:errcheck
	})
	go func() {
		if err := writeStdcopyFrame(serverConn, 1, "hook says hello\n"); err != nil {
			t.Error(err)
		}
		serverConn.Close() //nolint:errcheck
	}()
	h.api.EXPECT().ExecCreate(gomock.Any(), "c1", gomock.Any()).Return(client.ExecCreateResult{ID: "exec1"}, nil)
	h.api.EXPECT().ExecAttach(gomock.Any(), "exec1", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil)
	h.api.EXPECT().ExecInspect(gomock.Any(), "exec1", gomock.Any()).Return(client.ExecInspectResult{ExitCode: 0}, nil)

	plan := &Plan{}
	start := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:web:1", Cause: "start", Service: &web, Container: &ctr}, "start:web:1")
	start.Phase = PhaseStart
	post := plan.addNode(Operation{Type: OpRunPostStart, ResourceID: "service:web:1", Cause: "post_start hooks", Service: &web, Container: &ctr}, "start:web:1", start)
	post.Phase = PhaseStart
	created, _ := h.createdUpOf(t, project, plan)

	consumer := &testLogConsumer{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	options := interactiveUpOptions()
	options.Start.Attach = consumer
	done := make(chan error, 1)
	go func() { done <- h.svc.runInteractiveUp(ctx, created, options) }()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatalf("the session did not return; calls so far: %v", h.daemon.recorded())
	}

	var logged []string
	for _, lines := range consumer.logs {
		logged = append(logged, lines...)
	}
	assert.Assert(t, slices.Contains(logged, "hook says hello"), "the hook's output did not reach the session: %v", consumer.logs)
}

// TestUpInteractiveTranslatesAWaitTimeout pins what a depends_on wait that
// outlasts --wait-timeout makes of an interactive up: the dependency timeout
// the other commands report, not the composite error the executor builds to
// tell that wait's own deadline from any other.
func TestUpInteractiveTranslatesAWaitTimeout(t *testing.T) {
	h := newInteractiveUpHarness(t)
	project := interactiveUpProject()
	web := project.Services["web"]
	ctr := webContainerSummary()
	h.api.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).AnyTimes().
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:     "db-id",
			Name:   "/test-db-1",
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
			State:  &container.State{Status: container.StateRunning, Health: &container.Health{Status: container.Starting}},
		}}, nil)

	plan := &Plan{}
	wait := plan.addNode(Operation{Type: OpWaitCondition, ResourceID: "wait:db:service_healthy", Name: "db", Condition: types.ServiceConditionHealthy}, "")
	wait.Phase = PhaseStart
	start := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:web:1", Cause: "start", Service: &web, Container: &ctr}, "start:web:1", wait)
	start.Phase = PhaseStart
	created, exec := h.createdUpOf(t, project, plan)
	exec.waitTimeout = 50 * time.Millisecond
	exec.containersByService["db"] = Containers{{ID: "db-id", Names: []string{"/test-db-1"}, Labels: map[string]string{api.ServiceLabel: "db"}}}

	err := h.runInteractiveUp(t, created)

	assert.Error(t, err, "timeout waiting for dependencies")
	assert.Equal(t, h.daemon.indexOf("start"), -1, "web must not start before its dependency is healthy: %v", h.daemon.recorded())
}

// TestUpInteractiveFailedAttachEndsTheSession pins that an interactive up whose
// session cannot attach to the created containers fails with that error, starts
// nothing, and returns rather than waiting on the session it had begun to set
// up.
func TestUpInteractiveFailedAttachEndsTheSession(t *testing.T) {
	h := newInteractiveUpHarness(t)
	h.daemon.listErr = errors.New("cannot list the containers to attach to")

	err := h.up(t)

	assert.ErrorContains(t, err, "cannot list the containers to attach to")
	assert.Equal(t, h.daemon.indexOf("start"), -1, "nothing may start: %v", h.daemon.recorded())
}

// TestPrepareUpBoundsEveryWaitByTheWaitTimeout pins that --wait-timeout reaches
// the executor of an up, detached or interactive: it bounds each depends_on
// wait of the plan, even without --wait.
func TestPrepareUpBoundsEveryWaitByTheWaitTimeout(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)
	apiClient.EXPECT().ImageInspect(gomock.Any(), "alpine", gomock.Any()).AnyTimes().
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:alpine"}}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.VolumeListResult{}, nil)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).AnyTimes().Return(client.ContainerListResult{}, nil)

	options := api.UpOptions{Start: api.StartOptions{WaitTimeout: 42 * time.Second}}
	_, exec, _, err := svc.prepareUp(t.Context(), interactiveUpProject(), options)

	assert.NilError(t, err)
	assert.Equal(t, exec.waitTimeout, 42*time.Second)
}

// TestUpInteractiveExitsWithTheExitCodeOfTheServiceItIsToldTo pins what
// --exit-code-from does to an interactive up: the up ends with the exit code
// the container of the named service exited with, as a status error, where
// without it the up succeeds whatever the containers exited with.
func TestUpInteractiveExitsWithTheExitCodeOfTheServiceItIsToldTo(t *testing.T) {
	t.Run("with --exit-code-from", func(t *testing.T) {
		h := newInteractiveUpHarness(t)
		h.daemon.exitCode = "3"
		options := interactiveUpOptions()
		options.Start.ExitCodeFrom = "web"
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		err := h.upWithOptions(t, ctx, cancel, options)

		var status dockercli.StatusError
		assert.Assert(t, errors.As(err, &status), "err: %v", err)
		assert.Equal(t, status.StatusCode, 3)
	})

	t.Run("without it", func(t *testing.T) {
		h := newInteractiveUpHarness(t)
		h.daemon.exitCode = "3"

		assert.NilError(t, h.up(t))
	})
}
