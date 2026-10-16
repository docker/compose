/*
   Copyright 2020 Docker Compose CLI authors

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
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/go-cmp/cmp"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// recordedEvent is the projection of api.ContainerEvent the monitor tests
// assert on
type recordedEvent struct {
	eventType  int
	id         string
	source     string
	restarting bool
	exitCode   int
}

var cmpRecordedEvents = cmp.AllowUnexported(recordedEvent{})

func recordEvents(m *monitor) *[]recordedEvent {
	var got []recordedEvent
	m.withListener(func(e api.ContainerEvent) {
		got = append(got, recordedEvent{
			eventType:  e.Type,
			id:         e.ID,
			source:     e.Source,
			restarting: e.Restarting,
			exitCode:   e.ExitCode,
		})
	})
	return &got
}

func containerMessage(action events.Action, id, name, service string, extra map[string]string) events.Message {
	attributes := map[string]string{
		"name":                   name,
		api.ServiceLabel:         service,
		api.ContainerNumberLabel: "1",
	}
	for k, v := range extra {
		attributes[k] = v
	}
	return events.Message{
		Action: action,
		Actor:  events.Actor{ID: id, Attributes: attributes},
	}
}

func inspectResult(running, restarting bool) client.ContainerInspectResult {
	return client.ContainerInspectResult{
		Container: container.InspectResponse{
			State: &container.State{Running: running, Restarting: restarting},
		},
	}
}

// expectNowAsDaemonTime makes the mocked engine report a system time, as many
// times as the monitor asks: the test's clock, as a local daemon would.
func expectNowAsDaemonTime(apiClient *mocks.MockAPIClient) {
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
			return client.SystemInfoResult{Info: system.Info{SystemTime: time.Now().Format(time.RFC3339Nano)}}, nil
		}).AnyTimes()
}

func expectEventStream(apiClient *mocks.MockAPIClient, initial []container.Summary, capacity int) (chan events.Message, chan error) {
	expectNowAsDaemonTime(apiClient)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: initial}, nil)
	messages := make(chan events.Message, capacity)
	errs := make(chan error, 1)
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).
		Return(client.EventsResult{Messages: messages, Err: errs})
	return messages, errs
}

func TestMonitorStartLifecycle(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")
	got := recordEvents(m)

	messages, _ := expectEventStream(apiClient, []container.Summary{
		{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db"}},
	}, 10)

	gomock.InOrder(
		// c2 exits but is configured to restart on exit
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "c2", gomock.Any()).Return(inspectResult(false, true), nil),
		// c2 exits and restarts again, but this time the engine reports state
		// "running" instead of "restarting" (see moby/moby#45538)
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "c2", gomock.Any()).Return(inspectResult(true, false), nil),
		// c3 is already removed when we inspect it
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "c3", gomock.Any()).Return(client.ContainerInspectResult{}, errdefs.ErrNotFound),
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "c2", gomock.Any()).Return(inspectResult(false, false), nil),
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil),
	)

	// the monitor trims the default "<project>-<service>-<number>" name to "<service>-<number>"
	defaultName := getDefaultContainerName("p", "web", "1")
	messages <- containerMessage(events.ActionCreate, "c2", defaultName, "web", nil)
	messages <- containerMessage(events.ActionCreate, "c3", "custom-name", "job", map[string]string{api.ContainerReplaceLabel: "old"})
	messages <- containerMessage(events.ActionRestart, "c2", defaultName, "web", nil)
	messages <- containerMessage(events.ActionDie, "c2", defaultName, "web", map[string]string{"exitCode": "1"})
	messages <- containerMessage(events.ActionStart, "c2", defaultName, "web", nil)
	messages <- containerMessage(events.ActionDie, "c2", defaultName, "web", map[string]string{"exitCode": "1"})
	messages <- containerMessage(events.ActionStart, "c2", defaultName, "web", nil)
	messages <- containerMessage(events.ActionDie, "c3", "custom-name", "job", map[string]string{"exitCode": "0"})
	messages <- containerMessage(events.ActionDie, "c2", defaultName, "web", map[string]string{"exitCode": "1"})
	messages <- containerMessage(events.ActionDie, "c1", "c1-name", "db", map[string]string{"exitCode": "0"})

	err := m.Start(t.Context())
	assert.NilError(t, err)

	assert.DeepEqual(t, *got, []recordedEvent{
		{eventType: api.ContainerEventCreated, id: "c2", source: "web-1"},
		{eventType: api.ContainerEventRecreated, id: "c3", source: "custom-name"},
		{eventType: api.ContainerEventRestarted, id: "c2", source: "web-1"},
		{eventType: api.ContainerEventExited, id: "c2", source: "web-1", restarting: true, exitCode: 1},
		{eventType: api.ContainerEventStarted, id: "c2", source: "web-1", restarting: true},
		{eventType: api.ContainerEventExited, id: "c2", source: "web-1", restarting: true, exitCode: 1},
		{eventType: api.ContainerEventStarted, id: "c2", source: "web-1", restarting: true},
		{eventType: api.ContainerEventExited, id: "c3", source: "custom-name"},
		{eventType: api.ContainerEventExited, id: "c2", source: "web-1", exitCode: 1},
		{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
	}, cmpRecordedEvents)
}

func TestMonitorStartServiceFilter(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")
	m.withServices([]string{"web"})
	got := recordEvents(m)

	// the db container is not watched, so it doesn't count towards termination
	messages, _ := expectEventStream(apiClient, []container.Summary{
		{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web"}},
		{ID: "c2", Labels: map[string]string{api.ServiceLabel: "db"}},
	}, 10)

	// no ContainerInspect expectation for c2: its event must be ignored
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil)

	messages <- containerMessage(events.ActionDie, "c2", "c2-name", "db", map[string]string{"exitCode": "1"})
	messages <- containerMessage(events.ActionDie, "c1", "c1-name", "web", map[string]string{"exitCode": "0"})

	err := m.Start(t.Context())
	assert.NilError(t, err)

	assert.DeepEqual(t, *got, []recordedEvent{
		{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
	}, cmpRecordedEvents)
}

func TestMonitorStartNoContainers(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")

	expectEventStream(apiClient, nil, 1)

	err := m.Start(t.Context())
	assert.NilError(t, err)
}

func TestMonitorStartEventsError(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")

	_, errs := expectEventStream(apiClient, []container.Summary{
		{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db"}},
	}, 1)

	sentinel := errors.New("events stream failed")
	errs <- sentinel

	err := m.Start(t.Context())
	assert.ErrorIs(t, err, sentinel)
}

func TestMonitorStartContextCancelled(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")

	expectEventStream(apiClient, []container.Summary{
		{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db"}},
	}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := m.Start(ctx)
	assert.NilError(t, err)
}

func TestMonitorStartBadExitCode(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")

	messages, _ := expectEventStream(apiClient, []container.Summary{
		{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db"}},
	}, 1)

	messages <- containerMessage(events.ActionDie, "c1", "c1-name", "db", map[string]string{"exitCode": "not-a-number"})

	err := m.Start(t.Context())
	assert.ErrorContains(t, err, "not-a-number")
}

// monitorEvent builds an engine event for container "123"/service1, with the
// Actor.Attributes shape reported by the engine: compose labels plus the
// container name.
func monitorEvent(action events.Action) events.Message {
	attrs := containerLabels("service1", false)
	attrs["name"] = "testproject-service1-1"
	return events.Message{
		Type:   events.ContainerEventType,
		Action: action,
		Actor:  events.Actor{ID: "123", Attributes: attrs},
	}
}

// monitorDieEvent builds a die event, which the engine reports with an exit code.
func monitorDieEvent(exitCode int) events.Message {
	event := monitorEvent(events.ActionDie)
	event.Actor.Attributes["exitCode"] = strconv.Itoa(exitCode)
	return event
}

// newMonitorTestFixture wires a monitor against a mocked API client, with the
// goroutine-leak guard and the standard initial ContainerList expectation.
func newMonitorTestFixture(t *testing.T) (*monitor, *mocks.MockAPIClient) {
	t.Helper()
	ignoreExisting := goleak.IgnoreCurrent()
	t.Cleanup(func() {
		goleak.VerifyNone(t, ignoreExisting)
	})
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	apiMock := mocks.NewMockAPIClient(mockCtrl)

	expectNowAsDaemonTime(apiMock)
	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{testContainer("service1", "123", false)}}, nil)

	m := newMonitor(apiMock, strings.ToLower(testProject))
	return m, apiMock
}

// expectEvents makes the mocked engine deliver the given events, in order.
func expectEvents(apiMock *mocks.MockAPIClient, msgs ...events.Message) {
	ch := make(chan events.Message, len(msgs))
	for _, msg := range msgs {
		ch <- msg
	}
	apiMock.EXPECT().Events(gomock.Any(), gomock.Any()).
		Return(client.EventsResult{Messages: ch, Err: make(chan error)})
}

// expectInspects makes successive inspections of container "123" report the
// given states, in order.
func expectInspects(apiMock *mocks.MockAPIClient, states ...container.State) {
	calls := make([]any, 0, len(states))
	for _, state := range states {
		calls = append(calls, apiMock.EXPECT().
			ContainerInspect(gomock.Any(), "123", client.ContainerInspectOptions{}).
			Return(client.ContainerInspectResult{Container: container.InspectResponse{State: &state}}, nil))
	}
	gomock.InOrder(calls...)
}

// runMonitor starts the monitor under test in a goroutine and waits (with a
// timeout) for it to return, reporting the events it published. It fails the
// test if the monitor doesn't stop on its own, which is how an un-fixed
// monitor.Start reacts to stop/destroy events it doesn't know how to process:
// the tracked containers set never empties, so the loop blocks forever on the
// events channel.
func runMonitor(t *testing.T, m *monitor) ([]api.ContainerEvent, error) {
	t.Helper()
	var got []api.ContainerEvent
	m.withListener(func(e api.ContainerEvent) {
		got = append(got, e)
	})

	done := make(chan error, 1)
	go func() {
		done <- m.Start(t.Context())
	}()
	select {
	case err := <-done:
		return got, err
	case <-time.After(10 * time.Second):
		t.Fatal("monitor did not stop")
		return nil, nil
	}
}

// TestMonitorExitsOnDestroy pins the expectation that a destroy event (e.g. a
// container removed by `docker rm` or `docker compose rm` outside of a
// tracked lifecycle transition) drops the container from the tracked set
// without requiring any inspection, so the monitor loop terminates.
func TestMonitorExitsOnDestroy(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock, monitorEvent(events.ActionDestroy))

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 0)
}

// TestMonitorExitsWhenRestartingContainerStopped is the #13985 repro: a
// container configured to restart on failure dies (engine reports it as
// still "restarting"), then is explicitly stopped (e.g. `docker stop`)
// before the restart happens. The monitor must inspect on stop, observe the
// container is no longer restarting/running, and evict it so the loop
// terminates instead of waiting forever for a start event that never comes.
// It must also correct the optimistic `Restarting: true` carried by the
// die-time event: otherwise that stays the last word listeners ever get on
// this container, wrongly implying a restart is still coming (PR #13990
// review).
func TestMonitorExitsWhenRestartingContainerStopped(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock, monitorDieEvent(1), monitorEvent(events.ActionStop))
	expectInspects(apiMock,
		// on die: waiting for the restart policy to kick in
		container.State{Status: container.StateRestarting, Restarting: true, ExitCode: 1},
		// on stop: the restart loop got canceled
		container.State{Status: container.StateExited, ExitCode: 1},
	)

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].Type, api.ContainerEventExited)
	assert.Equal(t, got[0].Restarting, true)
	assert.Equal(t, got[0].ExitCode, 1)
	assert.Equal(t, got[1].Type, api.ContainerEventExited)
	assert.Equal(t, got[1].Restarting, false)
	assert.Equal(t, got[1].ExitCode, 1)
}

// TestMonitorKeepsRunningOnRestart is the #13161 guard: a container that
// dies and is restarted by the engine (watch/sync workflows trigger this via
// `docker restart`) must not be evicted by an intervening stop event that is
// merely part of the moby#45538 restart sequence (State reports
// Running=true while mid-ContainerRestart). The monitor must keep tracking
// it and still observe the subsequent start.
func TestMonitorKeepsRunningOnRestart(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock,
		monitorDieEvent(0),
		monitorEvent(events.ActionStop),
		monitorEvent(events.ActionStart),
		monitorDieEvent(1),
	)
	expectInspects(apiMock,
		// on die then on stop: mid-ContainerRestart, so still reported as running
		container.State{Status: container.StateRunning, Running: true},
		container.State{Status: container.StateRunning, Running: true},
		// on the final die: really gone
		container.State{Status: container.StateExited, ExitCode: 1},
	)

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 3)
	assert.Equal(t, got[0].Type, api.ContainerEventExited)
	assert.Equal(t, got[0].Restarting, true)
	assert.Equal(t, got[0].ExitCode, 0)
	assert.Equal(t, got[1].Type, api.ContainerEventStarted)
	assert.Equal(t, got[1].Restarting, true)
	assert.Equal(t, got[2].Type, api.ContainerEventExited)
	assert.Equal(t, got[2].Restarting, false)
	assert.Equal(t, got[2].ExitCode, 1)
}

// TestMonitorStopInspectNotFound covers a stop event racing a container's
// removal (e.g. `docker compose down` completing its stop+rm before this
// inspect runs): the inspect on stop returns NotFound, which must be
// tolerated (not treated as a fatal error). A container reaching a stop
// event has necessarily run, so its die is either already processed
// (restarting.Has(ctr.ID) — not the case here) or still queued right behind
// this stop; NotFound alone must not evict, or that pending die's real exit
// code would never reach listeners once containers empties. The monitor
// terminates once that die is drained instead.
func TestMonitorStopInspectNotFound(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock, monitorEvent(events.ActionStop), monitorDieEvent(137))
	gomock.InOrder(
		apiMock.EXPECT().ContainerInspect(gomock.Any(), "123", client.ContainerInspectOptions{}).
			Return(client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("no such container: 123")),
		apiMock.EXPECT().ContainerInspect(gomock.Any(), "123", client.ContainerInspectOptions{}).
			Return(client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("no such container: 123")),
	)

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].Type, api.ContainerEventExited)
	assert.Equal(t, got[0].ExitCode, 137)
}

// TestMonitorStopBeforeDiePreservesExitCode is the PR #13990 review fix: for
// a plain `docker stop` (no restart policy involved), the relative order of
// stop and die is not guaranteed (see the Start doc). If stop is processed
// first and evicts the last tracked container before its already-queued die
// is drained, the loop terminates (len(containers) == 0 is checked before
// reading the next event) and the real ContainerEventExited — carrying the
// exit code --exit-code-from and --abort-on-container-exit rely on — is lost.
func TestMonitorStopBeforeDiePreservesExitCode(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock, monitorEvent(events.ActionStop), monitorDieEvent(42))
	expectInspects(apiMock,
		// on stop: already exited, no restart pending, but its die hasn't
		// been processed yet
		container.State{Status: container.StateExited, ExitCode: 42},
		// on die: same definitive state
		container.State{Status: container.StateExited, ExitCode: 42},
	)

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].Type, api.ContainerEventExited)
	assert.Equal(t, got[0].Restarting, false)
	assert.Equal(t, got[0].ExitCode, 42)
}

// TestMonitorDieClearsStaleRestartingEntry is the other PR #13990 review nit:
// a stop landing before its die on a plain shutdown can find the container
// still reported as running (mid-transition) and mark it as restarting; the
// die that follows turns out definitive and must clear that entry too, or it
// lingers and would mislabel a later start event for the same ID as a
// policy-driven restart instead of a fresh one. A second, unrelated
// "keepalive" container is tracked alongside "target" so the loop doesn't
// terminate the moment target's own die empties its slot, letting the stale
// entry's effect on target's following start event be observed.
func TestMonitorDieClearsStaleRestartingEntry(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	m := newMonitor(apiClient, "p")
	got := recordEvents(m)

	messages, _ := expectEventStream(apiClient, []container.Summary{
		{ID: "target", Labels: map[string]string{api.ServiceLabel: "app"}},
		{ID: "keepalive", Labels: map[string]string{api.ServiceLabel: "app"}},
	}, 10)

	gomock.InOrder(
		// stop lands before die, mid-transition: still reported as running
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "target", gomock.Any()).Return(inspectResult(true, false), nil),
		// die turns out definitive
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "target", gomock.Any()).Return(inspectResult(false, false), nil),
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "keepalive", gomock.Any()).Return(inspectResult(false, false), nil),
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "target", gomock.Any()).Return(inspectResult(false, false), nil),
	)

	messages <- containerMessage(events.ActionStop, "target", "target-name", "app", nil)
	messages <- containerMessage(events.ActionDie, "target", "target-name", "app", map[string]string{"exitCode": "1"})
	messages <- containerMessage(events.ActionStart, "target", "target-name", "app", nil)
	messages <- containerMessage(events.ActionDie, "keepalive", "keepalive-name", "app", map[string]string{"exitCode": "0"})
	messages <- containerMessage(events.ActionDie, "target", "target-name", "app", map[string]string{"exitCode": "2"})

	err := m.Start(t.Context())
	assert.NilError(t, err)

	assert.DeepEqual(t, *got, []recordedEvent{
		{eventType: api.ContainerEventExited, id: "target", source: "target-name", exitCode: 1},
		{eventType: api.ContainerEventStarted, id: "target", source: "target-name"},
		{eventType: api.ContainerEventExited, id: "keepalive", source: "keepalive-name"},
		{eventType: api.ContainerEventExited, id: "target", source: "target-name", exitCode: 2},
	}, cmpRecordedEvents)
}

// fakeEventLog stands in for the engine's event stream with respect to the
// one property the monitor's startup depends on: an event emitted while
// nobody is subscribed is lost to a plain subscription, and only delivered
// to one that asks for it with Since -- the engine replays its buffered
// events newer than Since, then goes live.
//
// Events are stamped, and the system time reported, by the engine's own
// clock, which skew sets apart from the test's.
type fakeEventLog struct {
	mu      sync.Mutex
	entries []loggedEvent
	// since records the Since of every subscription, in order
	since []string

	// skew is how far the engine's clock is ahead of the local one
	skew time.Duration
	// infoErr, when set, is what the engine answers to Info
	infoErr error
	// systemTime, when set, is the SystemTime the engine reports instead of
	// its clock's (an empty one included)
	systemTime *string
	// reportedTime is the last SystemTime the engine reported
	reportedTime string
}

// setSystemTime makes the engine report the given system time.
func (l *fakeEventLog) setSystemTime(systemTime string) {
	l.systemTime = &systemTime
}

// now is the engine's clock.
func (l *fakeEventLog) now() time.Time {
	return time.Now().Add(l.skew)
}

// info answers the monitor's request for the engine's system information.
func (l *fakeEventLog) info() (client.SystemInfoResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.infoErr != nil {
		return client.SystemInfoResult{}, l.infoErr
	}
	if l.systemTime != nil {
		l.reportedTime = *l.systemTime
	} else {
		l.reportedTime = l.now().Format(time.RFC3339Nano)
	}
	return client.SystemInfoResult{Info: system.Info{SystemTime: l.reportedTime}}, nil
}

type loggedEvent struct {
	at  time.Time
	msg events.Message
}

func (l *fakeEventLog) emit(msg events.Message) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	msg.TimeNano = now.UnixNano()
	l.entries = append(l.entries, loggedEvent{at: now, msg: msg})
}

// subscribe opens a stream replaying the buffered events newer than
// opts.Since (none when it is empty), followed by the given live events.
func (l *fakeEventLog) subscribe(t *testing.T, opts client.EventsListOptions, live ...events.Message) client.EventsResult {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.since = append(l.since, opts.Since)

	messages := make(chan events.Message, len(l.entries)+len(live))
	if opts.Since != "" {
		since := parseEventsSince(t, opts.Since)
		for _, e := range l.entries {
			if !e.at.Before(since) {
				messages <- e.msg
			}
		}
	}
	for _, msg := range live {
		messages <- msg
	}
	return client.EventsResult{Messages: messages, Err: make(chan error)}
}

// parseEventsSince parses the "<seconds>.<nanoseconds>" form of the events
// API's since parameter.
func parseEventsSince(t *testing.T, since string) time.Time {
	t.Helper()
	sec, nsec, _ := strings.Cut(since, ".")
	// called from the monitor's goroutine: Check, not NilError, which would
	// FailNow off the test goroutine
	s, err := strconv.ParseInt(sec, 10, 64)
	assert.Check(t, err, "since %q", since)
	var n int64
	if nsec != "" {
		n, err = strconv.ParseInt(nsec, 10, 64)
		assert.Check(t, err, "since %q", since)
	}
	return time.Unix(s, n)
}

// monitorWithEventLog wires a monitor on a mocked engine whose container
// listing returns listed, after running during (the engine-side activity
// happening between the monitor's snapshot and its subscription), and whose
// event subscription is served by the returned log, followed by live.
func monitorWithEventLog(t *testing.T, listed []container.Summary, during func(*fakeEventLog), live ...events.Message) (*monitor, *mocks.MockAPIClient, *fakeEventLog) {
	t.Helper()
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	log := &fakeEventLog{}
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
			return log.info()
		})
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
			if during != nil {
				during(log)
			}
			return client.ContainerListResult{Items: listed}, nil
		})
	apiClient.EXPECT().Events(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts client.EventsListOptions) client.EventsResult {
			return log.subscribe(t, opts, live...)
		})
	return newMonitor(apiClient, "p"), apiClient, log
}

// startWithin runs the monitor and fails the test when it doesn't terminate
// on its own: a monitor which missed the event it waits for blocks forever.
func startWithin(t *testing.T, m *monitor) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- m.Start(t.Context()) }()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("monitor did not terminate: it never saw the event it was waiting for")
	}
}

// TestMonitorSeesEventsEmittedBeforeSubscription is the startup race of an
// `up` running its Create and Start phases back to back: the first container
// starts and exits right after the monitor takes its snapshot, before its
// event subscription is effective ("echo hi"). Without Since, the die is
// emitted to nobody, the monitor tracks the container forever, and
// --abort-on-container-exit / --exit-code-from never fire.
func TestMonitorSeesEventsEmittedBeforeSubscription(t *testing.T) {
	m, apiClient, _ := monitorWithEventLog(t,
		[]container.Summary{{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web"}}},
		func(log *fakeEventLog) {
			log.emit(containerMessage(events.ActionStart, "c1", "c1-name", "web", nil))
			log.emit(containerMessage(events.ActionDie, "c1", "c1-name", "web", map[string]string{"exitCode": "3"}))
		})
	got := recordEvents(m)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil)

	startWithin(t, m)

	assert.DeepEqual(t, *got, []recordedEvent{
		{eventType: api.ContainerEventStarted, id: "c1", source: "c1-name"},
		{eventType: api.ContainerEventExited, id: "c1", source: "c1-name", exitCode: 3},
	}, cmpRecordedEvents)
}

// TestMonitorReplayIsIdempotent covers the other side of asking the engine
// for events since a point BEFORE the container listing: some of the replayed
// events are already reflected in that listing. Each must leave the tracked
// set and the notifications the way a live delivery would have.
func TestMonitorReplayIsIdempotent(t *testing.T) {
	web := map[string]string{api.ServiceLabel: "web"}
	exit := func(code string) map[string]string { return map[string]string{"exitCode": code} }

	tests := []struct {
		name   string
		listed []container.Summary
		during func(*fakeEventLog)
		live   []events.Message
		// inspects are the successive ContainerInspect results
		inspects map[string][]client.ContainerInspectResult
		want     []recordedEvent
	}{
		{
			name: "die of a container the listing reports exited",
			// the listing is taken after the container exited: it is tracked
			// all the same, and only the replayed die gets it out of the set
			listed: []container.Summary{{ID: "c1", Labels: web, State: container.StateExited}},
			during: func(log *fakeEventLog) {
				log.emit(containerMessage(events.ActionStart, "c1", "c1-name", "web", nil))
				log.emit(containerMessage(events.ActionDie, "c1", "c1-name", "web", exit("0")))
			},
			inspects: map[string][]client.ContainerInspectResult{"c1": {inspectResult(false, false)}},
			want: []recordedEvent{
				{eventType: api.ContainerEventStarted, id: "c1", source: "c1-name"},
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
			},
		},
		{
			name: "start of a container the listing reports running",
			// the replayed start keeps the container tracked: it only ends
			// with the live die that follows
			listed: []container.Summary{{ID: "c1", Labels: web, State: container.StateRunning}},
			during: func(log *fakeEventLog) {
				log.emit(containerMessage(events.ActionStart, "c1", "c1-name", "web", nil))
			},
			live:     []events.Message{containerMessage(events.ActionDie, "c1", "c1-name", "web", exit("0"))},
			inspects: map[string][]client.ContainerInspectResult{"c1": {inspectResult(false, false)}},
			want: []recordedEvent{
				{eventType: api.ContainerEventStarted, id: "c1", source: "c1-name"},
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
			},
		},
		{
			name: "whole life of a container the listing doesn't know",
			// c1 came and went before the listing; c2 is what keeps the
			// monitor running
			listed: []container.Summary{{ID: "c2", Labels: web, State: container.StateRunning}},
			during: func(log *fakeEventLog) {
				log.emit(containerMessage(events.ActionCreate, "c1", "c1-name", "web", nil))
				log.emit(containerMessage(events.ActionStart, "c1", "c1-name", "web", nil))
				log.emit(containerMessage(events.ActionDie, "c1", "c1-name", "web", exit("0")))
				log.emit(containerMessage(events.ActionDestroy, "c1", "c1-name", "web", nil))
			},
			live: []events.Message{containerMessage(events.ActionDie, "c2", "c2-name", "web", exit("0"))},
			inspects: map[string][]client.ContainerInspectResult{
				"c1": {{}}, // removed since: NotFound, see below
				"c2": {inspectResult(false, false)},
			},
			want: []recordedEvent{
				{eventType: api.ContainerEventCreated, id: "c1", source: "c1-name"},
				{eventType: api.ContainerEventStarted, id: "c1", source: "c1-name"},
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
				{eventType: api.ContainerEventExited, id: "c2", source: "c2-name"},
			},
		},
		{
			name: "restart policy already in action",
			// die then start of a container the listing reports running
			// again: the replayed die finds it running and is reported as a
			// restart, as live delivery would, and the container stays
			// tracked until its final die
			listed: []container.Summary{{ID: "c1", Labels: web, State: container.StateRunning}},
			during: func(log *fakeEventLog) {
				log.emit(containerMessage(events.ActionDie, "c1", "c1-name", "web", exit("1")))
				log.emit(containerMessage(events.ActionStart, "c1", "c1-name", "web", nil))
			},
			live: []events.Message{containerMessage(events.ActionDie, "c1", "c1-name", "web", exit("0"))},
			inspects: map[string][]client.ContainerInspectResult{
				"c1": {inspectResult(true, false), inspectResult(false, false)},
			},
			want: []recordedEvent{
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name", restarting: true, exitCode: 1},
				{eventType: api.ContainerEventStarted, id: "c1", source: "c1-name", restarting: true},
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, apiClient, _ := monitorWithEventLog(t, tt.listed, tt.during, tt.live...)
			got := recordEvents(m)
			for id, results := range tt.inspects {
				var calls []any
				for _, res := range results {
					if res.Container.State == nil {
						calls = append(calls, apiClient.EXPECT().ContainerInspect(gomock.Any(), id, gomock.Any()).
							Return(client.ContainerInspectResult{}, errdefs.ErrNotFound))
						continue
					}
					calls = append(calls, apiClient.EXPECT().ContainerInspect(gomock.Any(), id, gomock.Any()).Return(res, nil))
				}
				gomock.InOrder(calls...)
			}

			startWithin(t, m)

			assert.DeepEqual(t, *got, tt.want, cmpRecordedEvents)
		})
	}
}

// TestMonitorSinceIsTheDaemonTime pins the clock the replay is relative to:
// the daemon's, read from its system info, not the client's. Events are
// stamped by the daemon, so a client clock that differs from it, as it does
// for a remote daemon, would miss an event emitted between the monitor's
// reading of the time and its subscription (clock behind), or replay what
// predates the monitor (clock ahead).
func TestMonitorSinceIsTheDaemonTime(t *testing.T) {
	for name, skew := range map[string]time.Duration{
		"daemon clock behind": -3 * time.Hour,
		"daemon clock ahead":  3 * time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			m, apiClient, log := monitorWithEventLog(t,
				[]container.Summary{{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web"}}},
				func(log *fakeEventLog) {
					log.emit(containerMessage(events.ActionDie, "c1", "c1-name", "web", map[string]string{"exitCode": "0"}))
				})
			log.skew = skew
			// an event of the daemon's past, from before the monitor is started
			log.emit(containerMessage(events.ActionDie, "old", "old-name", "web", map[string]string{"exitCode": "0"}))
			time.Sleep(time.Millisecond)
			got := recordEvents(m)
			apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil)

			startWithin(t, m)

			// the die emitted while the monitor was starting is replayed, and
			// not the one that predates it...
			assert.DeepEqual(t, *got, []recordedEvent{
				{eventType: api.ContainerEventExited, id: "c1", source: "c1-name"},
			}, cmpRecordedEvents)
			// ... because the subscription asks for what the daemon reported
			assert.Equal(t, len(log.since), 1)
			reported, err := time.Parse(time.RFC3339Nano, log.reportedTime)
			assert.NilError(t, err)
			assert.Check(t, parseEventsSince(t, log.since[0]).Equal(reported),
				"since %s is not the daemon time %s", log.since[0], log.reportedTime)
			assert.Check(t, time.Until(reported) > skew-time.Minute && time.Until(reported) < skew+time.Minute,
				"daemon time %s doesn't carry the clock skew %s", reported, skew)
		})
	}
}

// TestMonitorSinceUsesTheReportedSystemTime pins that the daemon's time is
// used as reported, whatever it is: here one deliberately far from the local
// clock, with a UTC offset of its own.
func TestMonitorSinceUsesTheReportedSystemTime(t *testing.T) {
	m, apiClient, log := monitorWithEventLog(t,
		[]container.Summary{{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web"}}},
		nil,
		containerMessage(events.ActionDie, "c1", "c1-name", "web", map[string]string{"exitCode": "0"}))
	log.setSystemTime("2001-02-03T04:05:06.789012345+05:30")
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil)

	startWithin(t, m)

	// 2001-02-03T04:05:06.789012345+05:30 is 2001-02-02T22:35:06.789012345Z
	assert.DeepEqual(t, log.since, []string{"981153306.789012345"})
}

// TestMonitorStartFailsWithoutDaemonTime: a daemon whose time can't be read
// has no sound starting point to replay events from, and the client's clock
// is no substitute: Start fails, explicitly, before anything else is asked of
// the engine (a strict mock: no listing, no subscription).
func TestMonitorStartFailsWithoutDaemonTime(t *testing.T) {
	unreachable := errors.New("daemon unreachable")
	tests := []struct {
		name       string
		systemTime string
		infoErr    error
		wantErr    string
	}{
		{name: "info fails", infoErr: unreachable, wantErr: "daemon unreachable"},
		{name: "system time is empty", wantErr: "no system time"},
		{name: "system time is not a time", systemTime: "yesterday-ish", wantErr: "yesterday-ish"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
			apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(
				client.SystemInfoResult{Info: system.Info{SystemTime: tt.systemTime}}, tt.infoErr)
			m := newMonitor(apiClient, "p")

			err := m.Start(t.Context())

			assert.ErrorContains(t, err, "reading the daemon time to subscribe to container events")
			assert.ErrorContains(t, err, tt.wantErr)
			if tt.infoErr != nil {
				assert.Check(t, errors.Is(err, tt.infoErr), "the engine's error is wrapped: %v", err)
			}
		})
	}
}

// TestMonitorStartCanceledBeforeDaemonTime: an interruption that makes the
// daemon's time unavailable is no failure, as for an interruption anywhere
// else in Start.
func TestMonitorStartCanceledBeforeDaemonTime(t *testing.T) {
	apiClient := mocks.NewMockAPIClient(gomock.NewController(t))
	apiClient.EXPECT().Info(gomock.Any(), gomock.Any()).Return(client.SystemInfoResult{}, context.Canceled)
	m := newMonitor(apiClient, "p")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.NilError(t, m.Start(ctx))
}

// TestMonitorReadDaemonTimeOnce: a caller that took the daemon's time ahead
// of Start (to fail before it starts any container) doesn't make Start ask
// again, and Start keeps the point taken then.
func TestMonitorReadDaemonTimeOnce(t *testing.T) {
	m, apiClient, log := monitorWithEventLog(t,
		[]container.Summary{{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web"}}},
		nil,
		containerMessage(events.ActionDie, "c1", "c1-name", "web", map[string]string{"exitCode": "0"}))
	log.setSystemTime("2001-02-03T04:05:06.789012345+05:30")
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "c1", gomock.Any()).Return(inspectResult(false, false), nil)

	assert.NilError(t, m.readDaemonTime(t.Context()))
	log.setSystemTime("2002-02-03T04:05:06Z") // would be seen by a second reading
	startWithin(t, m)

	assert.DeepEqual(t, log.since, []string{"981153306.789012345"})
}
