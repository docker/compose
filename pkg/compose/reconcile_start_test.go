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
		Summary:    container.Summary{ID: name + "-id", Names: []string{"/" + name}, State: state},
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
[1,3] -> #4 service:app:1, StartContainer, start [start:app:1] {start}
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

// An exceptional-state container (paused, dead, ...) gets a bare create-phase
// restart, not a create: its start-phase node targets the observed container
// itself and orders after that restart node.
func TestPlanStart_ExceptionalStateReplicaKeepsObservedTarget(t *testing.T) {
	service := types.ServiceConfig{Name: "app"}
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

	var createPhase, startPhase *PlanNode
	for _, n := range plan.Nodes {
		if n.Operation.Type != OpStartContainer {
			continue
		}
		if n.Phase == PhaseStart {
			startPhase = n
		} else {
			createPhase = n
		}
	}
	if createPhase == nil || startPhase == nil {
		t.Fatalf("plan misses expected start nodes (createPhase=%v startPhase=%v):\n%s", createPhase, startPhase, plan)
	}
	assert.Equal(t, startPhase.Operation.CreateNodeID, 0)
	assert.Assert(t, startPhase.Operation.Container != nil)
	assert.Equal(t, startPhase.Operation.Container.ID, "myproject-app-1-id")
	assert.Assert(t, slices.Contains(startPhase.DependsOn, createPhase))
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
