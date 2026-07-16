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
	"fmt"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
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

const (
	executorTestProjectName      = "test"
	executorTestNetworkKey       = "default"
	executorTestNetworkName      = "test_default"
	executorTestNetworkResource  = "network:" + executorTestNetworkKey
	executorTestNotFoundCause    = "not found"
	executorTestCreatedNetworkID = "net1"
	ipamOptionsKey               = "test"
	ipamOptionsValue             = "1"
)

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
	err := svc.executePlan(t.Context(), &types.Project{Name: executorTestProjectName}, emptyObservedState(executorTestProjectName), &Plan{})
	assert.NilError(t, err)
}

func TestExecutePlanCreateNetwork(t *testing.T) {
	svc, apiClient := newTestService(t)

	nw := types.NetworkConfig{Name: executorTestNetworkName}
	project := &types.Project{
		Name:     executorTestProjectName,
		Networks: types.Networks{executorTestNetworkKey: nw},
	}

	// ensureNetwork: inspect → not found, list → empty, create
	apiClient.EXPECT().NetworkInspect(gomock.Any(), executorTestNetworkName, gomock.Any()).
		Return(client.NetworkInspectResult{}, notFoundError{})
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).
		Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().NetworkCreate(gomock.Any(), executorTestNetworkName, gomock.Any()).
		Return(client.NetworkCreateResult{ID: executorTestCreatedNetworkID}, nil)

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: executorTestNetworkResource,
		Cause:      executorTestNotFoundCause,
		Name:       nw.Name,
		Network:    &nw,
	}, "")

	err := svc.executePlan(t.Context(), project, emptyObservedState(executorTestProjectName), plan)
	assert.NilError(t, err)
}

func TestExecutePlanCreateNetworkWithIPAMOptions(t *testing.T) {
	svc, apiClient := newTestService(t)

	nw := types.NetworkConfig{
		Name: executorTestNetworkName,
		Ipam: types.IPAMConfig{
			Options: types.Options{
				ipamOptionsKey: ipamOptionsValue,
			},
		},
	}
	project := &types.Project{
		Name:     executorTestProjectName,
		Networks: types.Networks{executorTestNetworkKey: nw},
	}

	apiClient.EXPECT().NetworkInspect(gomock.Any(), executorTestNetworkName, gomock.Any()).
		Return(client.NetworkInspectResult{}, notFoundError{})
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).
		Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().NetworkCreate(gomock.Any(), executorTestNetworkName, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
			assert.DeepEqual(t, opts.IPAM, &network.IPAM{
				Options: map[string]string{
					ipamOptionsKey: ipamOptionsValue,
				},
			})
			return client.NetworkCreateResult{ID: executorTestCreatedNetworkID}, nil
		})

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: executorTestNetworkResource,
		Cause:      executorTestNotFoundCause,
		Name:       nw.Name,
		Network:    &nw,
	}, "")

	err := svc.executePlan(t.Context(), project, emptyObservedState(executorTestProjectName), plan)
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
//
// Goes through newPlanExecutor + run (i.e. the same code path executePlan
// uses in production) so the test exercises the errgroup, done-channel
// wiring and group tracker — not a hand-rolled loop over executeNode.
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

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, observed)
	assert.NilError(t, exec.run(t.Context(), plan))

	assert.Equal(t, len(exec.containersByService["web"]), 0,
		"removed container should be dropped from the live view")
}

// TestExecutePlanConcurrentRemovesCacheCoherence stresses the cache mutex by
// scheduling N independent Stop+Remove pairs that the DAG lets the errgroup
// run concurrently. After the plan completes the cache must be empty, with no
// duplicates and no surviving entries — failure under -race would indicate a
// missing or incorrect lock around containersByService.
func TestExecutePlanConcurrentRemovesCacheCoherence(t *testing.T) {
	svc, apiClient := newTestService(t)

	const replicas = 5
	ctrs := make([]container.Summary, replicas)
	for i := range ctrs {
		ctrs[i] = container.Summary{
			ID:    fmt.Sprintf("c%d", i),
			Names: []string{fmt.Sprintf("/test-web-%d", i+1)},
			Labels: map[string]string{
				api.ServiceLabel:         "web",
				api.ContainerNumberLabel: fmt.Sprintf("%d", i+1),
			},
		}
	}

	// Each container gets exactly one Stop and one Remove. gomock matches by
	// any order across calls, so concurrent execution is fine.
	for i := range ctrs {
		apiClient.EXPECT().ContainerStop(gomock.Any(), ctrs[i].ID, gomock.Any()).
			Return(client.ContainerStopResult{}, nil)
		apiClient.EXPECT().ContainerRemove(gomock.Any(), ctrs[i].ID, gomock.Any()).
			Return(client.ContainerRemoveResult{}, nil)
	}

	webContainers := make([]ObservedContainer, replicas)
	for i := range ctrs {
		webContainers[i] = ObservedContainer{ID: ctrs[i].ID, Summary: ctrs[i]}
	}
	observed := &ObservedState{
		ProjectName: "test",
		Containers:  map[string][]ObservedContainer{"web": webContainers},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	// Build N independent Stop→Remove chains. The errgroup will fan them out
	// across goroutines that all hammer containersByService under the mutex.
	plan := &Plan{}
	for i := range ctrs {
		stop := plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: fmt.Sprintf("service:web:%d", i+1),
			Cause:      "scale down",
			Container:  &ctrs[i],
		}, "")
		plan.addNode(Operation{
			Type:       OpRemoveContainer,
			ResourceID: fmt.Sprintf("service:web:%d", i+1),
			Cause:      "scale down",
			Container:  &ctrs[i],
		}, "", stop)
	}

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, observed)
	assert.NilError(t, exec.run(t.Context(), plan))

	assert.Equal(t, len(exec.containersByService["web"]), 0,
		"all removed containers should be dropped from the live view")
}

// notFoundError implements the errdefs.ErrNotFound interface for test mocks.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}
