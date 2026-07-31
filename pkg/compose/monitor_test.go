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
	"strings"
	"testing"
	"time"

	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

const monitorCompletionTimeout = time.Second

func TestMonitorStopsAfterDestroyWithoutDie(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient, _ := prepareMocks(mockCtrl)
	projectName := strings.ToLower(testProject)
	eventsCh := make(chan events.Message)
	errCh := make(chan error)

	apiClient.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		All: true,
		Filters: projectFilter(projectName).Add("label",
			oneOffFilter(false),
			compose.ConfigHashLabel,
		),
	}).Return(client.ContainerListResult{
		Items: []containerType.Summary{
			testContainer("crasher", "crasher-container", false),
			testContainer("sleeper", "sleeper-container", false),
		},
	}, nil)
	apiClient.EXPECT().Events(gomock.Any(), client.EventsListOptions{
		Filters: projectFilter(projectName).Add("type", "container"),
	}).Return(client.EventsResult{Messages: eventsCh, Err: errCh})

	monitor := newMonitor(apiClient, projectName)
	done := make(chan error, 1)
	go func() {
		done <- monitor.Start(t.Context())
	}()

	eventsCh <- monitorEvent(events.ActionDestroy, "crasher", "crasher-container")
	eventsCh <- monitorEvent(events.ActionDestroy, "sleeper", "sleeper-container")

	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(monitorCompletionTimeout):
		t.Fatal("monitor did not stop after all containers were destroyed")
	}
}

func monitorEvent(action events.Action, service, id string) events.Message {
	attributes := containerLabels(service, false)
	attributes["name"] = id
	return events.Message{
		Action: action,
		Actor: events.Actor{
			ID:         id,
			Attributes: attributes,
		},
	}
}
