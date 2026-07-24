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
	"fmt"
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

	// createNetwork issues a plain NetworkCreate (divergence/reuse decisions are
	// made by the reconciler from the observed state, not here).
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
		Networks:    map[string][]ObservedNetwork{},
		Volumes:     map[string][]ObservedVolume{},
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
		Networks: map[string][]ObservedNetwork{},
		Volumes:  map[string][]ObservedVolume{},
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
		Networks:    map[string][]ObservedNetwork{},
		Volumes:     map[string][]ObservedVolume{},
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

// TestExecutePlanRecreateVolume drives the destructive core of a volume
// recreation — stop container → remove container → remove volume → create
// volume — end to end through the executor, asserting each Docker API call
// fires. The dependency edges force the destructive order: the volume can only
// be removed once the container referencing it is gone.
func TestExecutePlanRecreateVolume(t *testing.T) {
	svc, apiClient := newTestService(t)

	ctr := container.Summary{
		ID:    "c1",
		Names: []string{"/test-db-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "db",
			api.ContainerNumberLabel: "1",
		},
	}

	apiClient.EXPECT().ContainerStop(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil)
	apiClient.EXPECT().VolumeRemove(gomock.Any(), "recreate_data", gomock.Any()).
		Return(client.VolumeRemoveResult{}, nil)
	apiClient.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).
		Return(client.VolumeCreateResult{}, nil)

	vol := types.VolumeConfig{Name: "recreate_data", Driver: "local"}
	project := &types.Project{
		Name:    "recreate",
		Volumes: types.Volumes{"data": vol},
	}

	plan := &Plan{}
	stopNode := plan.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: "service:db:1",
		Cause:      "mounted volume config changed",
		Container:  &ctr,
	}, "")
	removeNode := plan.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: "service:db:1",
		Cause:      "mounted volume config changed",
		Container:  &ctr,
	}, "", stopNode)
	removeVolNode := plan.addNode(Operation{
		Type:       OpRemoveVolume,
		ResourceID: "volume:data",
		Cause:      "config hash diverged",
		Name:       vol.Name,
	}, "", removeNode)
	plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: "volume:data",
		Cause:      "recreate after config change",
		Name:       vol.Name,
		Volume:     &vol,
	}, "", removeVolNode)

	err := svc.executePlan(t.Context(), project, emptyObservedState("recreate"), plan)
	assert.NilError(t, err)
}

// TestExecutePlanRecreateNetwork drives a network recreation — stop container →
// disconnect → remove network → create network → connect — end to end through
// the executor, asserting each Docker API call fires. The container keeps its
// identity (it is reconnected, not recreated).
func TestExecutePlanRecreateNetwork(t *testing.T) {
	svc, apiClient := newTestService(t)

	ctr := container.Summary{
		ID:    "c1",
		Names: []string{"/recreate-web-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ContainerNumberLabel: "1",
		},
	}

	apiClient.EXPECT().ContainerStop(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().NetworkDisconnect(gomock.Any(), "recreate_frontend", gomock.Any()).
		Return(client.NetworkDisconnectResult{}, nil)
	apiClient.EXPECT().NetworkRemove(gomock.Any(), "recreate_frontend", gomock.Any()).
		Return(client.NetworkRemoveResult{}, nil)
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "recreate_frontend", gomock.Any()).
		Return(client.NetworkCreateResult{ID: "net2"}, nil)
	apiClient.EXPECT().NetworkConnect(gomock.Any(), "recreate_frontend", gomock.Any()).
		Return(client.NetworkConnectResult{}, nil)

	nw := types.NetworkConfig{Name: "recreate_frontend", Driver: "overlay"}
	project := &types.Project{
		Name:     "recreate",
		Networks: types.Networks{"frontend": nw},
	}

	plan := &Plan{}
	stopNode := plan.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: "service:web:1",
		Cause:      "network frontend config changed",
		Container:  &ctr,
	}, "")
	disconnectNode := plan.addNode(Operation{
		Type:       OpDisconnectNetwork,
		ResourceID: "service:web:1",
		Cause:      "network frontend recreate",
		Container:  &ctr,
		Name:       nw.Name,
	}, "", stopNode)
	removeNode := plan.addNode(Operation{
		Type:       OpRemoveNetwork,
		ResourceID: "network:frontend",
		Cause:      "config hash diverged",
		Name:       nw.Name,
	}, "", disconnectNode)
	createNode := plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:frontend",
		Cause:      "recreate after config change",
		Name:       nw.Name,
		Network:    &nw,
	}, "", removeNode)
	plan.addNode(Operation{
		Type:       OpConnectNetwork,
		ResourceID: "service:web:1",
		Cause:      "network frontend recreate",
		Container:  &ctr,
		Name:       nw.Name,
	}, "", createNode)

	err := svc.executePlan(t.Context(), project, emptyObservedState("recreate"), plan)
	assert.NilError(t, err)
}

// TestExecutePlanCreateNetworkConflictIsSuccess verifies that a NetworkCreate
// conflict (a concurrent up/run created the same network) is treated as success
// rather than surfacing as a hard failure.
func TestExecutePlanCreateNetworkConflictIsSuccess(t *testing.T) {
	svc, apiClient := newTestService(t)

	nw := types.NetworkConfig{Name: "test_default"}
	project := &types.Project{Name: "test", Networks: types.Networks{"default": nw}}

	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, conflictError{})

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Cause:      "not found",
		Name:       nw.Name,
		Network:    &nw,
	}, "")

	assert.NilError(t, svc.executePlan(t.Context(), project, emptyObservedState("test"), plan))
}

// TestExecRemoveNetworkBestEffort verifies that a best-effort network removal
// (old network on a rename) tolerates a conflict (still in use) but propagates
// any other error, while a mandatory removal propagates conflicts too.
func TestExecRemoveNetworkBestEffort(t *testing.T) {
	newExec := func(t *testing.T) (*planExecutor, *mocks.MockAPIClient) {
		svc, apiClient := newTestService(t)
		return svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test")), apiClient
	}

	t.Run("best-effort ignores conflict", func(t *testing.T) {
		exec, apiClient := newExec(t)
		apiClient.EXPECT().NetworkRemove(gomock.Any(), "old", gomock.Any()).Return(client.NetworkRemoveResult{}, conflictError{})
		err := exec.execRemoveNetwork(t.Context(), Operation{Type: OpRemoveNetwork, Name: "old", BestEffort: true})
		assert.NilError(t, err)
	})
	t.Run("best-effort propagates non-conflict", func(t *testing.T) {
		exec, apiClient := newExec(t)
		apiClient.EXPECT().NetworkRemove(gomock.Any(), "old", gomock.Any()).Return(client.NetworkRemoveResult{}, errors.New("transport failure"))
		err := exec.execRemoveNetwork(t.Context(), Operation{Type: OpRemoveNetwork, Name: "old", BestEffort: true})
		assert.ErrorContains(t, err, "transport failure")
	})
	t.Run("mandatory removal propagates conflict", func(t *testing.T) {
		exec, apiClient := newExec(t)
		apiClient.EXPECT().NetworkRemove(gomock.Any(), "old", gomock.Any()).Return(client.NetworkRemoveResult{}, conflictError{})
		err := exec.execRemoveNetwork(t.Context(), Operation{Type: OpRemoveNetwork, Name: "old", BestEffort: false})
		assert.Assert(t, err != nil, "mandatory removal must propagate conflict")
	})
}

// notFoundError implements the errdefs.ErrNotFound interface for test mocks.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}

// conflictError implements the errdefs.ErrConflict interface for test mocks.
type conflictError struct{}

func (conflictError) Error() string { return "conflict" }
func (conflictError) Conflict()     {}
