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
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func startScopeOptions(scope ReconcileScope) ReconcileOptions {
	options := defaultReconcileOptions()
	options.Scope = scope
	return options
}

func emptyObserved() *ObservedState {
	return &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string][]ObservedNetwork{},
		Volumes:     map[string][]ObservedVolume{},
	}
}

func observedServiceContainer(service string, number int, state container.ContainerState, hash string) ObservedContainer {
	name := "myproject-" + service + "-" + strconv.Itoa(number)
	return ObservedContainer{
		ID:         name + "-id",
		Name:       name,
		State:      state,
		ConfigHash: hash,
		Number:     number,
		Summary: container.Summary{
			ID:    name + "-id",
			Names: []string{"/" + name},
			State: state,
			// the reconciler reads these back from the raw summary (e.g.
			// nextContainerNumber): keep them consistent with the typed fields
			Labels: map[string]string{
				api.ServiceLabel:         service,
				api.ContainerNumberLabel: strconv.Itoa(number),
				api.ConfigHashLabel:      hash,
			},
		},
	}
}

// A fresh up plans Create and Start as one DAG: each start depends on its
// own create, and the service_started dependency is a plain edge — the
// dependent's start waits for the dependency's chain end, no wait node.
func TestPlanStart_FreshUpWithStartedDependency(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db"},
			"web": {
				Name: "web",
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionStarted, Required: true},
				},
			},
		},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, CreateContainer, no existing container
[1] -> #2 service:web:1, CreateContainer, no existing container
[1] -> #3 service:db:1, StartContainer, start [start:db:1] {start}
[2,3] -> #4 service:web:1, StartContainer, start [start:web:1] {start}
`)+"\n")
}

// A condition other than service_started materializes as one wait node per
// (service, condition), shared by every dependent; health is re-observed at
// execution time, the plan only encodes what to wait for.
func TestPlanStart_HealthyConditionDeduplicated(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db"},
			"web": {
				Name: "web",
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionHealthy, Required: true},
				},
			},
			"worker": {
				Name: "worker",
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionHealthy, Required: true},
				},
			},
		},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, CreateContainer, no existing container
[1] -> #2 service:web:1, CreateContainer, no existing container
[1] -> #3 service:worker:1, CreateContainer, no existing container
[1] -> #4 service:db:1, StartContainer, start [start:db:1] {start}
[4] -> #5 wait:db:service_healthy, WaitCondition, depends_on condition {start}
[2,5] -> #6 service:web:1, StartContainer, start [start:web:1] {start}
[3,5] -> #7 service:worker:1, StartContainer, start [start:worker:1] {start}
`)+"\n")
}

// pre_start runs once per service before the first replica start, only when
// no replica was running at observation; replicas start sequentially, each
// chain link (start, then post_start when declared) gating the next.
func TestPlanStart_HooksAndReplicaChain(t *testing.T) {
	two := 2
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"app": {
				Name:      "app",
				PreStart:  []types.ServiceHook{{}},
				PostStart: []types.ServiceHook{{}},
				Scale:     &two,
			},
		},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:app:1, CreateContainer, no existing container
[] -> #2 service:app:2, CreateContainer, no existing container
[1] -> #3 service:app:1, RunPreStart, pre_start hooks [start:app:1] {start}
[3] -> #4 service:app:1, StartContainer, start [start:app:1] {start}
[4] -> #5 service:app:1, RunPostStart, post_start hooks [start:app:1] {start}
[2,5] -> #6 service:app:2, StartContainer, start [start:app:2] {start}
[6] -> #7 service:app:2, RunPostStart, post_start hooks [start:app:2] {start}
`)+"\n")
}

// With a replica already running and untouched by the plan, pre_start is
// gated off and the running replica gets no node — only the non-running one
// starts, the imperative isNotRunning role expressed in the plan.
func TestPlanStart_RunningReplicaGatesPreStart(t *testing.T) {
	two := 2
	service := types.ServiceConfig{
		Name:     "app",
		PreStart: []types.ServiceHook{{}},
		Scale:    &two,
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": service},
	}
	hash, err := serviceHashWithResolvedRefs(service, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StateRunning, hash),
		observedServiceContainer("app", 2, container.StateExited, hash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:app:2, StartContainer, start [start:app:2] {start}
`)+"\n")
}

// A recreated replica's start-phase node resolves its container from the
// recreate chain's create node — not the rename node registered in
// containerNodes, whose execution stores no result — and orders after the
// chain's end.
func TestPlanStart_RecreatedReplicaTargetsCreateNode(t *testing.T) {
	service := types.ServiceConfig{Name: "app"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": service},
	}
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StateRunning, "stale-hash"),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var create, rename, start *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpCreateContainer:
			create = n
		case n.Operation.Type == OpRenameContainer:
			rename = n
		case n.Operation.Type == OpStartContainer && n.Phase == PhaseStart:
			start = n
		}
	}
	if create == nil || rename == nil || start == nil {
		t.Fatalf("plan misses expected nodes (create=%v rename=%v start=%v):\n%s", create, rename, start, plan)
	}
	assert.Equal(t, start.Operation.CreateNodeID, create.ID)
	assert.Assert(t, start.Operation.Container == nil)
	assert.Assert(t, slices.Contains(start.DependsOn, rename))
}

// An exceptional-state container (paused, dead, ...) gets its bare
// create-phase restart and NOTHING in the start phase: the imperative engine
// finds it running when the start phase looks, so it neither re-starts nor
// injects — and its being running gates pre_start for the whole service.
func TestPlanStart_ExceptionalStateReplicaCountsAsRunning(t *testing.T) {
	service := types.ServiceConfig{
		Name:     "app",
		PreStart: []types.ServiceHook{{Command: []string{"echo", "hi"}}},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": service},
	}
	hash, err := serviceHashWithResolvedRefs(service, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StatePaused, hash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var createPhase *PlanNode
	for _, n := range plan.Nodes {
		if n.Phase == PhaseStart {
			t.Fatalf("no start-phase node expected for a create-phase-restarted replica, got %s:\n%s", n.Operation.Type, plan)
		}
		if n.Operation.Type == OpStartContainer {
			createPhase = n
		}
	}
	assert.Assert(t, createPhase != nil, "the historical bare restart must remain:\n%s", plan)
}

// Dependency conditions are re-verified even when nothing has to start — the
// imperative engine calls waitDependencies for every visited service before
// looking at what to start, so an up with everything running still fails on
// an unhealthy required dependency. The dependent's visit end (the wait) also
// orders whoever depends on IT via service_started.
func TestPlanStart_AllRunningStillWaitsOnConditions(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	app := types.ServiceConfig{
		Name:      "app",
		DependsOn: types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}},
	}
	web := types.ServiceConfig{
		Name:      "web",
		DependsOn: types.DependsOnConfig{"app": {Condition: types.ServiceConditionStarted, Required: true}},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "app": app, "web": web},
	}
	observed := emptyObserved()
	for _, svc := range []types.ServiceConfig{db, app, web} {
		hash, err := serviceHashWithResolvedRefs(svc, nil)
		assert.NilError(t, err)
		observed.Containers[svc.Name] = []ObservedContainer{
			observedServiceContainer(svc.Name, 1, container.StateRunning, hash),
		}
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var wait *PlanNode
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpWaitCondition {
			wait = n
		}
	}
	if wait == nil {
		t.Fatalf("expected the db:service_healthy wait despite everything running:\n%s", plan)
	}
	assert.Equal(t, wait.Operation.Name, "db")
	assert.Equal(t, wait.Operation.Condition, types.ServiceConditionHealthy)
	assert.Assert(t, !wait.Operation.BestEffort)
}

// Imperative parity (startService): under scope Start, a scale>0 service
// with no container at all fails the plan the way `compose start` fails.
func TestPlanStart_NoContainerToStartFails(t *testing.T) {
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": types.ServiceConfig{Name: "app"}},
	}

	_, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeStart), noPrompt)
	assert.ErrorContains(t, err, `service "app" has no container to start`)
}

// A replica condemned by scale-down must never receive a start-phase node:
// the imperative engine only starts what survives the convergence — and the
// condemned replica does not count as running for the pre_start gating.
func TestPlanStart_ScaleDownTargetIsNeverStarted(t *testing.T) {
	one := 1
	service := types.ServiceConfig{
		Name:   "app",
		Deploy: &types.DeployConfig{Replicas: &one},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": service},
	}
	hash, err := serviceHashWithResolvedRefs(service, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StateRunning, hash),
		// the excess replica is exited: without the condemned filter it
		// would fall through to the "observed, not running" start path
		observedServiceContainer("app", 2, container.StateExited, hash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	for _, n := range plan.Nodes {
		if n.Phase == PhaseStart {
			t.Fatalf("no start-phase node expected (replica 1 runs, replica 2 is condemned), got %s %s:\n%s", n.Operation.Type, n.Operation.ResourceID, plan)
		}
		if n.Operation.Type == OpRemoveContainer {
			assert.Equal(t, n.Operation.ResourceID, "service:app:2")
		}
	}
}

// A running container the create phase stops WITHOUT recreating (network
// recreate) is down when the start phase looks: the plan restarts it —
// ordered after its last reconnect — and it does not gate pre_start, exactly
// like the imperative engine restarting it from its second snapshot.
func TestStartPhaseReplicas_StoppedByPlanRestarts(t *testing.T) {
	stop := &PlanNode{ID: 1, Operation: Operation{Type: OpStopContainer}}
	reconnect := &PlanNode{ID: 2, Operation: Operation{Type: OpConnectNetwork}}
	observed := emptyObserved()
	hash, err := serviceHashWithResolvedRefs(types.ServiceConfig{Name: "app"}, nil)
	assert.NilError(t, err)
	oc := observedServiceContainer("app", 1, container.StateRunning, hash)
	observed.Containers["app"] = []ObservedContainer{oc}

	r := &reconciler{
		observed:       observed,
		containerNodes: map[string]map[int]*PlanNode{},
		removedByPlan:  map[string]bool{},
		stoppedByPlan:  map[string]*PlanNode{oc.ID: stop},
		connectNodes:   map[string][]*PlanNode{oc.ID: {reconnect}},
	}

	replicas, anyRunning := r.startPhaseReplicas(types.ServiceConfig{Name: "app"})
	assert.Assert(t, !anyRunning, "a stopped-by-plan container must not gate pre_start")
	if len(replicas) != 1 {
		t.Fatalf("expected the stopped-by-plan container to be restarted, got %d replicas", len(replicas))
	}
	assert.Equal(t, replicas[0].after, reconnect, "the restart orders after the last reconnect")
	assert.Assert(t, replicas[0].container != nil)
}

// plannedReplica leaves the target unresolved on an unexpected registration,
// so execution fails with a clean error instead of a plan-time panic.
func TestPlannedReplicaUnexpectedNodeLeavesTargetUnresolved(t *testing.T) {
	node := &PlanNode{ID: 7, Operation: Operation{Type: OpConnectNetwork}}
	rep := plannedReplica("app", 1, node)
	assert.Equal(t, rep.resID, "service:app:1")
	assert.Equal(t, rep.createNodeID, 0)
	assert.Assert(t, rep.container == nil)
	assert.Equal(t, rep.after, node)
}

// Replica start order is numeric, not lexicographic: with 10+ replicas,
// replica 2 starts before replica 10.
func TestPlanStart_ReplicaOrderIsNumeric(t *testing.T) {
	eleven := 11
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"app": {Name: "app", Scale: &eleven},
		},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var order []string
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpStartContainer {
			order = append(order, n.Operation.ResourceID)
		}
	}
	expected := make([]string, 0, 11)
	for i := 1; i <= 11; i++ {
		expected = append(expected, "service:app:"+strconv.Itoa(i))
	}
	assert.DeepEqual(t, order, expected)
}

// Scope Start plans only the start phase over observed containers — the
// future `compose start`: no convergence, exited containers start in
// dependency order, running ones are left alone.
func TestPlanStart_StartOnlyScope(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	web := types.ServiceConfig{
		Name: "web",
		DependsOn: types.DependsOnConfig{
			"db": {Condition: types.ServiceConditionStarted, Required: true},
		},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "web": web},
	}
	dbHash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	webHash, err := serviceHashWithResolvedRefs(web, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{observedServiceContainer("db", 1, container.StateExited, dbHash)}
	observed.Containers["web"] = []ObservedContainer{observedServiceContainer("web", 1, container.StateCreated, webHash)}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, StartContainer, start [start:db:1] {start}
[1] -> #2 service:web:1, StartContainer, start [start:web:1] {start}
`)+"\n")
}

// An optional (required: false) condition marks the shared wait node
// best-effort — a missing dependency is skipped, not fatal; one required
// dependent upgrades the node for everyone.
func TestPlanStart_OptionalConditionIsBestEffort(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db"},
			"web": {
				Name: "web",
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionHealthy, Required: false},
				},
			},
		},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var wait *PlanNode
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpWaitCondition {
			wait = n
		}
	}
	assert.Assert(t, wait != nil)
	assert.Assert(t, wait.Operation.BestEffort)

	// a second dependent requiring the same condition upgrades the node
	project.Services["worker"] = types.ServiceConfig{
		Name: "worker",
		DependsOn: types.DependsOnConfig{
			"db": {Condition: types.ServiceConditionHealthy, Required: true},
		},
	}
	plan, err = reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)
	wait = nil
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpWaitCondition {
			wait = n
		}
	}
	assert.Assert(t, wait != nil)
	assert.Assert(t, !wait.Operation.BestEffort)
}

// The default scope keeps yesterday's plans byte-identical: no start-phase
// node ever appears unless a caller opts in.
func TestPlanStart_DefaultScopeIsInert(t *testing.T) {
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": {Name: "app"}},
	}

	plan, err := reconcile(t.Context(), project, emptyObserved(), defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	for _, n := range plan.Nodes {
		assert.Assert(t, n.Phase == PhaseCreate)
	}
	assert.Equal(t, plan.String(), "[] -> #1 service:app:1, CreateContainer, no existing container\n")
}
