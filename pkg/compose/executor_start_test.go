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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func newRecordingTestService(t *testing.T) (*composeService, *mocks.MockAPIClient, *recordingEventProcessor) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	recorder := &recordingEventProcessor{}
	svc, err := NewComposeService(cli, WithEventProcessor(recorder))
	assert.NilError(t, err)
	return svc.(*composeService), apiClient, recorder
}

func startPhaseNode(plan *Plan, op Operation, group string, deps ...*PlanNode) *PlanNode {
	node := plan.addNode(op, group, deps...)
	node.Phase = PhaseStart
	return node
}

// A start-phase node on an observed container starts it through the engine
// and emits the imperative event pair, Starting then Started.
func TestExecuteStartPhase_ObservedContainer(t *testing.T) {
	svc, apiClient, recorder := newRecordingTestService(t)

	ctr := container.Summary{
		ID:    "c1",
		Names: []string{"/test-web-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ContainerNumberLabel: "1",
		},
	}
	apiClient.EXPECT().ContainerStart(gomock.Any(), "c1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	web := types.ServiceConfig{Name: "web"}
	plan := &Plan{}
	startPhaseNode(plan, Operation{
		Type:       OpStartContainer,
		ResourceID: "service:web:1",
		Cause:      "start",
		Service:    &web,
		Container:  &ctr,
		Name:       "test-web-1",
	}, "start:web:1")

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), plan)
	assert.NilError(t, err)
	assert.DeepEqual(t, recorder.summary(), []string{
		"Container test-web-1: Starting",
		"Container test-web-1: Started",
	})
}

// A start-phase node for a replica the same plan materialized resolves its
// target from the create node's result — the CreateNodeID mechanism.
func TestExecuteStartPhase_TargetFromCreateNode(t *testing.T) {
	svc, apiClient, _ := newRecordingTestService(t)

	apiClient.EXPECT().ContainerStart(gomock.Any(), "created-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	web := types.ServiceConfig{Name: "web"}
	plan := &Plan{}
	startPhaseNode(plan, Operation{
		Type:         OpStartContainer,
		ResourceID:   "service:web:1",
		Cause:        "start",
		Service:      &web,
		Name:         "test-web-1",
		CreateNodeID: 7,
	}, "start:web:1")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"))
	exec.pctx.set(7, operationResult{ContainerID: "created-id", ContainerName: "test-web-1"})
	assert.NilError(t, exec.run(t.Context(), plan))
}

// A best-effort wait (every dependent optional) absorbs a missing dependency
// as a Skipped event; a required one fails without touching the engine.
func TestExecuteStartPhase_WaitConditionMissingDependency(t *testing.T) {
	svc, _, recorder := newRecordingTestService(t)

	plan := &Plan{}
	startPhaseNode(plan, Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "depends_on condition",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
		BestEffort: true,
	}, "")

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), plan)
	assert.NilError(t, err)
	assert.DeepEqual(t, recorder.summary(), []string{"Service db: Skipped: no container to wait for"})

	required := &Plan{}
	startPhaseNode(required, Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "depends_on condition",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	err = svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), required)
	assert.ErrorContains(t, err, `required dependency "db" has no container to wait for`)
}

// execWaitCondition delegates the polling to the imperative waitDependency
// primitive: the Waiting and Healthy events are the exact vocabulary users
// see today.
func TestExecuteStartPhase_WaitHealthyDelegates(t *testing.T) {
	svc, apiClient, recorder := newRecordingTestService(t)

	db := container.Summary{
		ID:    "db1",
		Names: []string{"/test-db-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "db",
			api.ContainerNumberLabel: "1",
		},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db1", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:     "db1",
			Name:   "/test-db-1",
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Healthy},
			},
		}}, nil).MinTimes(1)

	observed := emptyObservedState("test")
	observed.Containers["db"] = []ObservedContainer{{ID: "db1", Name: "test-db-1", Number: 1, State: container.StateRunning, Summary: db}}

	plan := &Plan{}
	startPhaseNode(plan, Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "depends_on condition",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, observed, plan)
	assert.NilError(t, err)
	assert.DeepEqual(t, recorder.summary(), []string{
		"Container test-db-1: Waiting",
		"Container test-db-1: Healthy",
	})
}

// The recreate group's event name is the observed container's canonical
// name, even when the create node — carrying the temporary recreate name —
// appears first in the group (regression: TestRestartWithDependencies).
func TestGroupEventNameIgnoresTemporaryName(t *testing.T) {
	old := container.Summary{
		ID:    "8cdaa8cc3322",
		Names: []string{"/test-web-1"},
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ContainerNumberLabel: "1",
		},
	}
	plan := &Plan{}
	create := plan.addNode(Operation{
		Type: OpCreateContainer, ResourceID: "service:web:1", Name: "8cdaa8cc3322_test-web-1",
	}, "recreate:web:1")
	plan.addNode(Operation{
		Type: OpStopContainer, ResourceID: "service:web:1", Container: &old,
	}, "recreate:web:1", create)

	exec := &planExecutor{}
	groups := exec.buildGroupTracker(plan)
	assert.Equal(t, groups.groups["recreate:web:1"].eventName, "Container test-web-1")
}

// The start group closes on the chain's last node, so Started is emitted
// after post_start hooks ran — word-for-word the imperative sequence.
func TestStartGroupEmitsStartedAfterPostStart(t *testing.T) {
	web := types.ServiceConfig{Name: "web"}
	plan := &Plan{}
	start := startPhaseNode(plan, Operation{
		Type: OpStartContainer, ResourceID: "service:web:1", Service: &web, Name: "test-web-1",
	}, "start:web:1")
	post := startPhaseNode(plan, Operation{
		Type: OpRunPostStart, ResourceID: "service:web:1", Service: &web, Name: "test-web-1",
	}, "start:web:1", start)

	exec := &planExecutor{}
	groups := exec.buildGroupTracker(plan)
	recorder := &recordingEventProcessor{}

	groups.onNodeStart(start, recorder)
	groups.onNodeDone(start, recorder)
	assert.DeepEqual(t, recorder.summary(), []string{"Container test-web-1: Starting"})

	groups.onNodeStart(post, recorder)
	groups.onNodeDone(post, recorder)
	assert.DeepEqual(t, recorder.summary(), []string{
		"Container test-web-1: Starting",
		"Container test-web-1: Started",
	})
}
