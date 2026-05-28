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
	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), &Plan{})
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

	err := svc.executePlan(t.Context(), project, emptyObservedState("test"), plan)
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

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), plan)
	assert.NilError(t, err)
}

// emptyObservedState returns an ObservedState with no containers/networks/volumes,
// suitable for executor tests that don't exercise service-reference resolution.
func emptyObservedState(project string) *ObservedState {
	return &ObservedState{
		ProjectName: project,
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}
}

// TestExecutePlanRemoveContainerDropsFromCache verifies that after a container
// is removed, subsequent service-reference resolution does not see its stale ID.
// Without this guarantee, a recreate followed by a dependent's create can pick
// up the just-removed container, depending on the canonical-name sort order.
func TestExecutePlanRemoveContainerDropsFromCache(t *testing.T) {
	svc, apiClient := newTestService(t)

	oldCtr := container.Summary{
		ID:    "old-id",
		Names: []string{"/test-web-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ContainerNumberLabel: "1",
		},
	}

	apiClient.EXPECT().ContainerStop(gomock.Any(), "old-id", gomock.Any()).
		Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "old-id", gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil)

	observed := &ObservedState{
		ProjectName: "test",
		Containers: map[string][]ObservedContainer{
			"web": {{ID: "old-id", Summary: oldCtr}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan := &Plan{}
	stopNode := plan.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: "service:web:1",
		Cause:      "scale down",
		Container:  &oldCtr,
	}, "")
	plan.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: "service:web:1",
		Cause:      "scale down",
		Container:  &oldCtr,
	}, "", stopNode)

	// Seed and run the plan via the same code path as production.
	// After completion, the cache must no longer contain old-id.
	exec := &planExecutor{
		compose:             svc,
		project:             &types.Project{Name: "test"},
		pctx:                &reconciliationContext{results: map[int]operationResult{}},
		containersByService: observed.containersByService(),
	}
	for _, node := range plan.Nodes {
		assert.NilError(t, exec.executeNode(t.Context(), node))
	}

	assert.Equal(t, len(exec.containersByService["web"]), 0,
		"removed container should be dropped from the live view")
}

// notFoundError implements the errdefs.ErrNotFound interface for test mocks.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}
