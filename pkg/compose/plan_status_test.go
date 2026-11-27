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
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/internal/progress"
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
	plan.AddNode(planning.Operation{Type: planning.OpCreateVolume, ResourceID: "volume:data", Cause: "mounted volume config changed", Reason: "config changed", Name: vol.Name, Volume: &vol}, "", first)

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
	assert.Equal(t, seenWhileCreating.Resources[1].Message, "create: config changed")

	final := tr.Status()
	assert.DeepEqual(t, final.Resources, []api.ResourceStatus{
		{Kind: api.KindNetwork, Name: "test_default", State: api.StatusCreated, Level: api.Done, Action: "create"},
		{Kind: api.KindVolume, Name: "test_data", State: api.StatusCreated, Level: api.Done, Action: "create"},
	})
	assert.DeepEqual(t, final.Plan, []api.PlanStep{
		{ID: 1, Operation: "create_network", Resource: "Network test_default", State: api.StepSucceeded},
		{ID: 2, Operation: "create_volume", Resource: "Volume test_data", Reason: "config changed", DependsOn: []int{1}, State: api.StepSucceeded},
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
	assert.Equal(t, describePending([]string{"recreate"}, "config changed"), "recreate: config changed")
	assert.Equal(t, describePending(nil, ""), "")
	assert.Equal(t, operationName(planning.OpCreateHookContainer), "create_hook_container")
	assert.Equal(t, operationName(planning.OpWaitCondition), "wait_condition")
}

// The names of the operations are a contract of the status: every operation of
// the plan has one, and it is not the debug form of its type.
func TestEveryOperationHasANameInTheStatus(t *testing.T) {
	known := 0
	for op := planning.OperationType(0); op < 100; op++ {
		if strings.HasPrefix(op.String(), "Unknown(") {
			continue
		}
		known++
		name, ok := stepOperations[op]
		assert.Assert(t, ok, "%s has no name in the status", op)
		assert.Assert(t, name != "" && name == strings.ToLower(name) && !strings.ContainsAny(name, " -"), "%q is not snake case", name)
	}
	assert.Equal(t, known, len(stepOperations), "a name for an operation that does not exist")
	assert.Equal(t, operationName(planning.OperationType(99)), "unknown")
}

// capturePublisher records what a plan publishes.
type capturePublisher struct {
	steps    []api.PlanStep
	rows     []api.ResourceStatus
	outcomes map[int]api.PlanStep
	deferred []int
}

func (c *capturePublisher) Plan(steps []api.PlanStep, rows []api.ResourceStatus) {
	c.steps, c.rows = steps, rows
}

func (c *capturePublisher) Step(id int, state api.StepState, message string) {
	if c.outcomes == nil {
		c.outcomes = map[int]api.PlanStep{}
	}
	c.outcomes[id] = api.PlanStep{ID: id, State: state, Message: message}
}

func (c *capturePublisher) Defer(ids ...int) { c.deferred = append(c.deferred, ids...) }

func TestPublishPlanNamesWhatEachResourceWillGoThrough(t *testing.T) {
	svc, _ := newTestService(t)
	web := types.ServiceConfig{Name: "web"}
	project := &types.Project{Name: "test", Services: types.Services{"web": web}}

	plan := &planning.Plan{}
	network := plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Name: "test_default"}, "")
	create := plan.AddNode(planning.Operation{
		Type: planning.OpCreateContainer, ResourceID: "service:web:1", Name: "test-web-1", Service: &web, Cause: "no existing container",
	}, "", network)
	plan.AddNode(planning.Operation{
		Type: planning.OpStartContainer, ResourceID: "service:web:1", Service: &web, CreateNodeID: create.ID, Cause: "start",
	}, "start:service:web:1", create)

	pub := &capturePublisher{}
	exec := svc.newPlanExecutor(project, emptyObservedState("test"))
	publishPlan(pub, plan, exec.buildGroupTracker(plan))

	assert.DeepEqual(t, pub.rows, []api.ResourceStatus{
		{Kind: api.KindNetwork, Name: "test_default", Message: "create", Action: "create"},
		{Kind: api.KindContainer, Name: "test-web-1", Service: "web", Message: "create, start", Action: "create, start"},
	})
	assert.Equal(t, len(pub.steps), 3)
	assert.Equal(t, pub.steps[2].Operation, "start_container")
	assert.Equal(t, pub.steps[2].Resource, "Container test-web-1")
	assert.DeepEqual(t, pub.steps[2].DependsOn, []int{create.ID})
}

func TestContainersToRemoveListsWhatDownWillStopAndRemove(t *testing.T) {
	project := &types.Project{Name: "prj", Services: types.Services{"web": {Name: "web"}, "db": {Name: "db"}}}
	running := testContainer("web", "c1", false)
	running.State = "running"
	exited := testContainer("db", "c2", false)
	exited.State = "exited"
	orphan := testContainer("gone", "c3", false)
	orphan.State = "exited"
	containers := Containers{running, exited, orphan}

	rows := containersToRemove(project, containers, false)
	assert.Equal(t, len(rows), 2, "an orphan is left alone without --remove-orphans: %v", rows)
	byService := map[string]api.ResourceStatus{}
	for _, r := range rows {
		assert.Equal(t, r.Kind, api.KindContainer)
		byService[r.Service] = r
	}
	assert.Equal(t, byService["web"].Message, "stop, remove")
	assert.Equal(t, byService["web"].Action, "stop, remove")
	assert.Equal(t, byService["db"].Message, "remove")

	assert.Equal(t, len(containersToRemove(project, containers, true)), 3)
}

// `compose --profile debug up -d` then `compose down`: debug is disabled by its
// profile, and down leaves its container alone, even with --remove-orphans.
func TestContainersToRemoveLeavesTheContainersOfDisabledServices(t *testing.T) {
	project := &types.Project{
		Name:             "prj",
		Services:         types.Services{"web": {Name: "web"}},
		DisabledServices: types.Services{"debug": {Name: "debug"}},
	}
	web := testContainer("web", "c1", false)
	debug := testContainer("debug", "c2", false)
	debug.State = "running"

	for _, removeOrphans := range []bool{false, true} {
		rows := containersToRemove(project, Containers{web, debug}, removeOrphans)
		assert.Equal(t, len(rows), 1, "removeOrphans=%v: %v", removeOrphans, rows)
		assert.Equal(t, rows[0].Service, "web")
	}
}

// The rows are in the order down reaches them: what nothing depends on first,
// then what it depended on, and the orphans last.
func TestContainersToRemoveAreInTheOrderDownReachesThem(t *testing.T) {
	dependsOn := func(names ...string) types.WorkloadSpec {
		deps := types.DependsOnConfig{}
		for _, name := range names {
			deps[name] = types.ServiceDependency{}
		}
		return types.WorkloadSpec{DependsOn: deps}
	}
	project := &types.Project{Name: "prj", Services: types.Services{
		"db":  {Name: "db"},
		"api": {Name: "api", WorkloadSpec: dependsOn("db")},
		"web": {Name: "web", WorkloadSpec: dependsOn("api")},
		"abc": {Name: "abc"},
		// a diamond: the longest chain counts
		"left":  {Name: "left", WorkloadSpec: dependsOn("db")},
		"top":   {Name: "top", WorkloadSpec: dependsOn("left", "api", "abc")},
		"dummy": {Name: "dummy", WorkloadSpec: dependsOn("not-a-service-of-the-project")},
	}}
	var containers Containers
	for _, service := range []string{"db", "abc", "web", "gone", "api", "left", "top", "dummy"} {
		containers = append(containers, testContainer(service, "c-"+service, false))
	}

	var order []string
	for _, r := range containersToRemove(project, containers, true) {
		order = append(order, r.Service)
	}
	// top depends on web's chain (via api: db) and on left (db), abc; the
	// depth is the longest chain: db 0, abc 0, dummy 0 (an unknown dependency
	// does not count), api 1, left 1, web 2, top 2
	assert.DeepEqual(t, order, []string{"top", "web", "api", "left", "abc", "db", "dummy", "gone"})
}

func TestStopAndRemoveOfAnObservedContainerAreOfItsService(t *testing.T) {
	svc, _ := newTestService(t)
	project := &types.Project{Name: "test"}
	old := testContainer("web", "old", false)

	plan := &planning.Plan{}
	stop := plan.AddNode(planning.Operation{Type: planning.OpStopContainer, ResourceID: "service:web:1", Container: &old, Reason: "scale down"}, "")
	plan.AddNode(planning.Operation{Type: planning.OpRemoveContainer, ResourceID: "service:web:1", Container: &old}, "", stop)

	pub := &capturePublisher{}
	exec := svc.newPlanExecutor(project, emptyObservedState("test"))
	publishPlan(pub, plan, exec.buildGroupTracker(plan))

	assert.Equal(t, len(pub.rows), 1)
	assert.Equal(t, pub.rows[0].Service, "web", "a plan row of a container that is only observed has its service")
	assert.Equal(t, pub.rows[0].Message, "stop, remove: scale down")
	assert.Equal(t, pub.steps[0].Service, "web")
	assert.Equal(t, pub.steps[0].Reason, "scale down")
}

func TestHowAnOperationEndedIsToldToTheStatus(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name      string
		started   bool
		err       error
		succeeded bool
		want      api.StepState
		message   string
	}{
		{name: "succeeded", started: true, succeeded: true, want: api.StepSucceeded},
		{name: "a wait that ended without its condition", started: true, want: api.StepSkipped, message: "interrupted"},
		{name: "canceled while acting", started: true, err: context.Canceled, want: api.StepSkipped, message: "interrupted"},
		{name: "never started because a dependency failed", err: boom, want: api.StepSkipped, message: "dependency failed"},
		{name: "never started because canceled", err: context.Canceled, want: api.StepSkipped, message: "interrupted"},
		{name: "never started because the deadline of the caller passed", err: context.DeadlineExceeded, want: api.StepSkipped, message: "interrupted"},
		{
			// the dependency failed on its --wait-timeout, which is not the caller's deadline
			name: "never started because a wait it depends on timed out",
			err:  fmt.Errorf("%w: %w", errWaitTimeout, context.DeadlineExceeded), want: api.StepSkipped, message: "dependency failed",
		},
		{name: "failed", started: true, err: boom, want: api.StepFailed, message: "boom"},
		{
			// a --wait-timeout is a failure of the wait, which is a deadline too
			name: "ran out of time", started: true, want: api.StepFailed,
			err: fmt.Errorf("%w: %w", errWaitTimeout, context.DeadlineExceeded), message: "timeout waiting for dependencies: context deadline exceeded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub := &capturePublisher{}
			rs := &runState{status: pub, succeeded: func(int) bool { return tc.succeeded }}
			rs.publishOutcome(&planning.PlanNode{ID: 7}, tc.started, tc.err)
			assert.DeepEqual(t, pub.outcomes[7], api.PlanStep{ID: 7, State: tc.want, Message: tc.message})
		})
	}
}

// A plan that cannot run leaves nothing behind in the status.
func TestAnInvalidPlanIsNotPublished(t *testing.T) {
	svc, _ := newTestService(t)
	events := &statusEvents{}
	svc.events = events

	plan := &planning.Plan{}
	start := plan.AddNode(planning.Operation{Type: planning.OpStartContainer, ResourceID: "service:web:1"}, "")
	start.Phase = planning.PhaseStart
	plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Name: "n"}, "", start)

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"))
	_, err := exec.begin(plan)
	assert.ErrorContains(t, err, "invalid plan")
	assert.Assert(t, !events.published)
}

func TestTheStartPhaseIsDeferredToTheStatus(t *testing.T) {
	svc, _ := newTestService(t)
	events := &statusEvents{}
	svc.events = events

	plan := &planning.Plan{}
	create := plan.AddNode(planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:default", Name: "n"}, "")
	start := plan.AddNode(planning.Operation{Type: planning.OpStartContainer, ResourceID: "service:web:1"}, "", create)
	start.Phase = planning.PhaseStart

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"))
	run, err := exec.begin(plan)
	assert.NilError(t, err)
	run.deferStart()
	assert.DeepEqual(t, events.deferred, []int{start.ID})
}

func TestPublishPlanKeepsARecreatedContainerOnItsOwnRow(t *testing.T) {
	svc, _ := newTestService(t)
	web := types.ServiceConfig{Name: "web"}
	project := &types.Project{Name: "test", Services: types.Services{"web": web}}
	old := testContainer("web", "old", false)

	plan := &planning.Plan{}
	create := plan.AddNode(planning.Operation{
		Type: planning.OpCreateContainer, ResourceID: "service:web:1", Name: "abcdef123456_test-web-1", Service: &web, Cause: "config changed (tmpName)", Reason: "config changed",
	}, "recreate:web:1")
	stop := plan.AddNode(planning.Operation{Type: planning.OpStopContainer, ResourceID: "service:web:1", Container: &old, Service: &web}, "recreate:web:1", create)
	plan.AddNode(planning.Operation{Type: planning.OpStartContainer, ResourceID: "service:web:1", Service: &web, CreateNodeID: create.ID}, "start:service:web:1", stop)

	pub := &capturePublisher{}
	exec := svc.newPlanExecutor(project, emptyObservedState("test"))
	publishPlan(pub, plan, exec.buildGroupTracker(plan))

	oldRow := getContainerProgressName(old)
	assert.Equal(t, len(pub.rows), 1, "the temporary name of the replacement has no row of its own: %v", pub.rows)
	assert.Equal(t, pub.rows[0].ID(), oldRow)
	assert.Equal(t, pub.rows[0].Message, "recreate: config changed")
	assert.Equal(t, pub.rows[0].Action, "recreate")
	assert.Equal(t, pub.steps[2].Resource, oldRow)
}

// statusEvents is an event processor that keeps a status, as the tracker of the
// backend does, recording what is published to it.
type statusEvents struct {
	progress.Noop
	capturePublisher
	published bool
}

func (s *statusEvents) Plan(steps []api.PlanStep, rows []api.ResourceStatus) {
	s.published = true
	s.capturePublisher.Plan(steps, rows)
}

func TestDownOfSomeServicesIsNotAnnounced(t *testing.T) {
	project := &types.Project{Name: "prj", Services: types.Services{"web": {Name: "web"}, "db": {Name: "db"}}}
	containers := Containers{testContainer("web", "c1", false), testContainer("db", "c2", false)}

	events := &statusEvents{}
	svc := &composeService{events: events}

	svc.publishRemovals(project, containers, api.DownOptions{Services: []string{"web"}})
	assert.Assert(t, !events.published, "which containers go with some services depends on what depends on them")

	svc.publishRemovals(project, containers, api.DownOptions{})
	assert.Assert(t, events.published)
	assert.Equal(t, len(events.rows), 2)
}
