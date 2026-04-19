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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// noopEventProcessor discards all events.
type noopEventProcessor struct{}

func (noopEventProcessor) Start(_ context.Context, _ string) {}
func (noopEventProcessor) On(_ ...api.Resource)              {}
func (noopEventProcessor) Done(_ string, _ bool)             {}

func newTestService(t *testing.T) (*composeService, *mocks.MockAPIClient) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	svc, err := NewComposeService(cli, WithEventProcessor(noopEventProcessor{}))
	assert.NilError(t, err)
	return svc.(*composeService), apiClient
}

func TestExecutePlanEmpty(t *testing.T) {
	svc, _ := newTestService(t)
	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, &Plan{})
	assert.NilError(t, err)
}

func TestExecutePlanCreateNetwork(t *testing.T) {
	svc, apiClient := newTestService(t)

	nw := types.NetworkConfig{Name: "test_default"}
	project := &types.Project{
		Name:     "test",
		Networks: types.Networks{"default": nw},
	}

	// ensureNetwork: inspect → not found, list → empty, create
	apiClient.EXPECT().NetworkInspect(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkInspectResult{}, notFoundError{})
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).
		Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{ID: "net1"}, nil)

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Cause:      "not found",
		Name:       nw.Name,
		Network:    &nw,
	}, "")

	err := svc.executePlan(t.Context(), project, plan)
	assert.NilError(t, err)
}

func TestExecutePlanStopRemoveContainer(t *testing.T) {
	svc, apiClient := newTestService(t)

	ctr := container.Summary{
		ID:    "c1",
		Names: []string{"/test-web-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ContainerNumberLabel: "1",
		},
	}

	apiClient.EXPECT().ContainerStop(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil)

	plan := &Plan{}
	stopNode := plan.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: "service:web:1",
		Cause:      "scale down",
		Container:  &ctr,
	}, "")
	plan.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: "service:web:1",
		Cause:      "scale down",
		Container:  &ctr,
	}, "", stopNode)

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, plan)
	assert.NilError(t, err)
}

// notFoundError implements the errdefs.ErrNotFound interface for test mocks.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}
