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
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/internal/status"
	"github.com/docker/compose/v5/pkg/api"
)

func trackerOf(t *testing.T, svc *composeService) *status.Tracker {
	t.Helper()
	tr, ok := svc.events.(*status.Tracker)
	assert.Assert(t, ok, "the service keeps the status of its operations")
	return tr
}

func TestExecutePlanPublishesPlanAndOutcomes(t *testing.T) {
	svc, apiClient := newTestService(t)
	tr := trackerOf(t, svc)

	nw := types.NetworkConfig{Name: "test_default"}
	vol := types.VolumeConfig{Name: "test_data"}
	project := &types.Project{Name: "test", Networks: types.Networks{"default": nw}, Volumes: types.Volumes{"data": vol}}

	var seenWhileCreating api.ProjectStatus
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		DoAndReturn(func(any, any, any) (client.NetworkCreateResult, error) {
			seenWhileCreating = tr.Status()
			return client.NetworkCreateResult{ID: "net1"}, nil
		})
	apiClient.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Return(client.VolumeCreateResult{}, nil)

	plan := &planning.Plan{}
	first := plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Cause: "not found", Name: nw.Name, Network: &nw}, "")
	plan.AddNode(planning.Operation{Type: planning.OpCreateVolume, ResourceID: "volume:data", Cause: "mounted volume config changed", Name: vol.Name, Volume: &vol}, "", first)

	assert.NilError(t, svc.executePlan(t.Context(), project, emptyObservedState("test"), plan))

	// the plan was known before the first operation acted
	assert.Equal(t, len(seenWhileCreating.Plan), 2)
	var pending []string
	for _, r := range seenWhileCreating.Resources {
		if r.Pending() {
			pending = append(pending, r.ID())
		}
	}
	assert.DeepEqual(t, pending, []string{"Volume test_data"})
	assert.Equal(t, seenWhileCreating.Resources[1].Message, "create: mounted volume config changed")

	final := tr.Status()
	assert.DeepEqual(t, final.Resources, []api.ResourceStatus{
		{Kind: api.KindNetwork, Name: "test_default", State: api.StatusCreated, Level: api.Done},
		{Kind: api.KindVolume, Name: "test_data", State: api.StatusCreated, Level: api.Done},
	})
	assert.DeepEqual(t, final.Plan, []api.PlanStep{
		{ID: 1, Operation: "create_network", Resource: "Network test_default", Reason: "not found", State: api.StepSucceeded},
		{ID: 2, Operation: "create_volume", Resource: "Volume test_data", Reason: "mounted volume config changed", DependsOn: []int{1}, State: api.StepSucceeded},
	})
}

func TestExecutePlanPublishesAFailure(t *testing.T) {
	svc, apiClient := newTestService(t)
	tr := trackerOf(t, svc)

	nw := types.NetworkConfig{Name: "test_default"}
	project := &types.Project{Name: "test", Networks: types.Networks{"default": nw}}
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, errors.New("pool overlaps"))

	plan := &planning.Plan{}
	plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")

	err := svc.executePlan(t.Context(), project, emptyObservedState("test"), plan)
	assert.ErrorContains(t, err, "pool overlaps")

	final := tr.Status()
	assert.Equal(t, final.Plan[0].State, api.StepFailed)
	assert.Assert(t, final.Plan[0].Message != "")
	assert.Equal(t, final.Resources[0].Level, api.Error)
}

func TestExecutePlanSkipsWhatADependencyFailedFor(t *testing.T) {
	svc, apiClient := newTestService(t)
	tr := trackerOf(t, svc)

	nw := types.NetworkConfig{Name: "test_default"}
	vol := types.VolumeConfig{Name: "test_data"}
	project := &types.Project{Name: "test", Networks: types.Networks{"default": nw}, Volumes: types.Volumes{"data": vol}}
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, errors.New("boom"))

	plan := &planning.Plan{}
	first := plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")
	plan.AddNode(planning.Operation{Type: planning.OpCreateVolume, ResourceID: "volume:data", Name: vol.Name, Volume: &vol}, "", first)

	assert.Assert(t, svc.executePlan(t.Context(), project, emptyObservedState("test"), plan) != nil)

	final := tr.Status()
	assert.Equal(t, final.Plan[0].State, api.StepFailed)
	assert.Equal(t, final.Plan[1].State, api.StepSkipped)
	assert.DeepEqual(t, final.Plan[1].DependsOn, []int{1})
	assert.Assert(t, final.Resources[1].Pending(), "what was never reached stays pending: %v", final.Resources[1])
}

func TestServiceWithoutStatusPublishesNowhere(t *testing.T) {
	svc := &composeService{events: nil}
	// must not panic
	svc.statusPublisher().Plan([]api.PlanStep{{ID: 1}}, nil)
	svc.statusPublisher().Step(1, api.StepRunning, "")
}

func TestPlannedActionsAndReasons(t *testing.T) {
	assert.Equal(t, describePending([]string{"create", "start"}, ""), "create, start")
	assert.Equal(t, describePending([]string{"recreate"}, "config hash diverged"), "recreate: config hash diverged")
	assert.Equal(t, describePending(nil, ""), "")
	assert.Equal(t, userReason("config hash diverged"), "config hash diverged")
	assert.Equal(t, userReason("replaced by #3"), "")
	assert.Equal(t, userReason("config changed (tmpName)"), "")
	assert.Equal(t, operationName(planning.OpCreateHookContainer), "create_hook_container")
	assert.Equal(t, operationName(planning.OpWaitCondition), "wait_condition")
}
