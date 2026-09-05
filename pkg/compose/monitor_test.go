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
	"testing"

	"github.com/containerd/errdefs"
	"github.com/google/go-cmp/cmp"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
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
