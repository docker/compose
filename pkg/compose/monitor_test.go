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
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/go-cmp/cmp"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
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

func expectEventStream(apiClient *mocks.MockAPIClient, initial []container.Summary, capacity int) (chan events.Message, chan error) {
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
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0].Type, api.ContainerEventExited)
	assert.Equal(t, got[0].Restarting, true)
	assert.Equal(t, got[0].ExitCode, 1)
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
// removal: the inspect on stop returns NotFound, which must be tolerated
// (not treated as a fatal error) and the container evicted so the monitor
// terminates.
func TestMonitorStopInspectNotFound(t *testing.T) {
	m, apiMock := newMonitorTestFixture(t)
	expectEvents(apiMock, monitorEvent(events.ActionStop))
	apiMock.EXPECT().ContainerInspect(gomock.Any(), "123", client.ContainerInspectOptions{}).
		Return(client.ContainerInspectResult{}, errdefs.ErrNotFound.WithMessage("no such container: 123"))

	got, err := runMonitor(t, m)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 0)
}
