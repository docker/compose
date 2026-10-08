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

// serviceWithDeps builds a ServiceConfig with its depends_on in assignment
// style: DependsOn is a promoted field (WorkloadSpec), which struct literals
// cannot name before go1.27.
func serviceWithDeps(name string, deps types.DependsOnConfig) types.ServiceConfig {
	svc := types.ServiceConfig{Name: name}
	svc.DependsOn = deps
	return svc
}

// A fresh up plans Create and Start as one DAG: each start depends on its
// own create, and the service_started dependency is a plain edge — the
// dependent's start waits for the dependency's chain end, no wait node.
func TestPlanStart_FreshUpWithStartedDependency(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db"},
			"web": serviceWithDeps("web", types.DependsOnConfig{
				"db": {Condition: types.ServiceConditionStarted, Required: true},
			}),
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
			"web": serviceWithDeps("web", types.DependsOnConfig{
				"db": {Condition: types.ServiceConditionHealthy, Required: true},
			}),
			"worker": serviceWithDeps("worker", types.DependsOnConfig{
				"db": {Condition: types.ServiceConditionHealthy, Required: true},
			}),
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
				PreStart:  []types.PreStartHook{{}},
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
[1,2] -> #3 hook:app:pre_start:0, CreateHookContainer, pre_start hook
[1,3] -> #4 service:app:1, RunPreStart, pre_start hooks [start:app:1] {start}
[4] -> #5 service:app:1, StartContainer, start [start:app:1] {start}
[5] -> #6 service:app:1, RunPostStart, post_start hooks [start:app:1] {start}
[2,6] -> #7 service:app:2, StartContainer, start [start:app:2] {start}
[7] -> #8 service:app:2, RunPostStart, post_start hooks [start:app:2] {start}
`)+"\n")
}

// With a replica already running and untouched by the plan, pre_start is
// gated off and the running replica gets no node — only the non-running one
// starts, the imperative isNotRunning role expressed in the plan.
func TestPlanStart_RunningReplicaGatesPreStart(t *testing.T) {
	two := 2
	service := types.ServiceConfig{
		Name:     "app",
		PreStart: []types.PreStartHook{{}},
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
	hook := types.PreStartHook{}
	hook.Command = []string{"echo", "hi"}
	service := types.ServiceConfig{
		Name:     "app",
		PreStart: []types.PreStartHook{hook},
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
	app := serviceWithDeps("app", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	web := serviceWithDeps("web", types.DependsOnConfig{"app": {Condition: types.ServiceConditionStarted, Required: true}})
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

// Imperative parity (startService): a service with scale: 0 is still visited
// -- its depends_on conditions are evaluated before the no-op, so an unhealthy
// dependency still fails `up`/`start` and whatever is ordered after the
// service is ordered after those prerequisites -- and under scope Start the
// replicas it already owns are started. Only deploy.replicas: 0 is an
// unconditional no-op. Having no container is never an error for it.
func TestPlanStart_ScaleZeroIsStillVisited(t *testing.T) {
	zero := 0
	db := types.ServiceConfig{Name: "db"}
	scaled := serviceWithDeps("app", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	scaled.Scale = &zero
	deployed := serviceWithDeps("job", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	deployed.Deploy = &types.DeployConfig{Replicas: &zero}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "app": scaled, "job": deployed},
	}
	dbHash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{observedServiceContainer("db", 1, container.StateRunning, dbHash)}

	// no container for the scale-zero service: its dependency wait is planned
	// and nothing is an error; deploy.replicas: 0 plans nothing at all
	for _, scope := range []ReconcileScope{ScopeStart, ScopeCreateStart} {
		plan, err := reconcile(t.Context(), project, observed, startScopeOptions(scope), noPrompt)
		assert.NilError(t, err)
		waits := 0
		for _, n := range plan.Nodes {
			switch n.Operation.Type {
			case OpWaitCondition:
				waits++
				assert.Equal(t, n.Operation.Name, "db")
			case OpStartContainer, OpCreateContainer:
				t.Fatalf("no container expected for scale-zero services, got %s %s:\n%s", n.Operation.Type, n.Operation.ResourceID, plan)
			}
		}
		assert.Equal(t, waits, 1, "one deduplicated db wait, from app only:\n%s", plan)
	}

	// scope Start also starts the replicas the scale-zero service still owns
	appHash, err := serviceHashWithResolvedRefs(scaled, nil)
	assert.NilError(t, err)
	observed.Containers["app"] = []ObservedContainer{observedServiceContainer("app", 1, container.StateExited, appHash)}
	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)
	var started []string
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpStartContainer {
			started = append(started, n.Operation.ResourceID)
		}
	}
	assert.DeepEqual(t, started, []string{"service:app:1"})

	// the create phase removes every replica of a scale-zero service before
	// the start phase looks: nothing to start under CreateStart
	plan, err = reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)
	for _, n := range plan.Nodes {
		assert.Assert(t, n.Operation.Type != OpStartContainer, "unexpected start node:\n%s", plan)
	}
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

// A depends_on naming a service that is neither declared nor disabled is
// the same hard error the imperative engine raises (shouldWaitForDependency
// propagates it) — reachable via `compose start` without a compose file,
// where projectFromName never populates DisabledServices.
func TestPlanStart_MissingDependencyFails(t *testing.T) {
	app := serviceWithDeps("app", types.DependsOnConfig{"ghost": {Condition: types.ServiceConditionHealthy, Required: true}})
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": app},
	}
	hash, err := serviceHashWithResolvedRefs(app, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StateExited, hash),
	}

	_, err = reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.ErrorContains(t, err, "ghost")

	// ordering parity: with NO container at all under scope Start, the
	// dependency error still wins over errNoContainerToStart — the
	// imperative startService waits on dependencies first
	_, err = reconcile(t.Context(), project, emptyObserved(), startScopeOptions(ScopeStart), noPrompt)
	assert.ErrorContains(t, err, "ghost")
}

// Imperative parity (startService): waitDependencies runs for every service,
// providers included — a provider's own depends_on must produce its wait
// nodes instead of being skipped with the container-start machinery.
func TestPlanStart_ProviderWaitsOnDependencies(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	db.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	prov := serviceWithDeps("prov", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "prov": prov},
	}
	hash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{
		observedServiceContainer("db", 1, container.StateRunning, hash),
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
		t.Fatalf("expected a db:service_healthy wait for the provider's depends_on:\n%s", plan)
	}
	assert.Equal(t, wait.Operation.Name, "db")
	assert.Equal(t, wait.Operation.Condition, types.ServiceConditionHealthy)
}

// TestPlanStart_ProviderConsumerWaitsForDeployment is a regression test for
// a Copilot review finding on #14290: once ScopeCreateStart became the real,
// exercised path for detached up, planProviderStart's startChainEnds for a
// provider WITH its own depends_on was just that dependency's wait node --
// dropping the provider's own OpRunProvider (deployment) node entirely. A
// downstream consumer of the provider could therefore start while the
// provider plugin/relay deployment was still running, a barrier the old
// create-then-start two-phase sequence guaranteed for free.
func TestPlanStart_ProviderConsumerWaitsForDeployment(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	db.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	prov := serviceWithDeps("prov", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	consumer := serviceWithDeps("consumer", types.DependsOnConfig{"prov": {Condition: types.ServiceConditionStarted, Required: true}})
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "prov": prov, "consumer": consumer},
	}
	dbHash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	consumerHash, err := serviceHashWithResolvedRefs(consumer, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{
		observedServiceContainer("db", 1, container.StateRunning, dbHash),
	}
	observed.Containers["consumer"] = []ObservedContainer{
		observedServiceContainer("consumer", 1, container.StateExited, consumerHash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var providerNode, consumerStartNode *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpRunProvider:
			providerNode = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:consumer:1":
			consumerStartNode = n
		}
	}
	if providerNode == nil {
		t.Fatalf("expected an OpRunProvider node for prov:\n%s", plan)
	}
	if consumerStartNode == nil {
		t.Fatalf("expected a start node for consumer:\n%s", plan)
	}
	assert.Assert(t, slices.Contains(consumerStartNode.DependsOn, providerNode),
		"consumer's start must depend on the provider's own deployment node, not just its depends_on wait:\n%s", plan)
}

// TestPlanStart_ExceptionalStateConsumerWaitsForRestart is the sibling
// regression to TestPlanStart_ProviderConsumerWaitsForDeployment for an
// ordinary (non-provider) service: the same either/or bug in
// planServiceStart's len(replicas) == 0 branch dropped the service's own
// create-phase bare-restart node (TestPlanStart_ExceptionalStateReplicaCountsAsRunning)
// from startChainEnds whenever the service also had its own depends_on,
// keeping only the dependency wait. A downstream service_started consumer
// could then start before a paused/dead service's only container was
// actually running again.
func TestPlanStart_ExceptionalStateConsumerWaitsForRestart(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	db.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	app := serviceWithDeps("app", types.DependsOnConfig{"db": {Condition: types.ServiceConditionHealthy, Required: true}})
	consumer := serviceWithDeps("consumer", types.DependsOnConfig{"app": {Condition: types.ServiceConditionStarted, Required: true}})
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "app": app, "consumer": consumer},
	}
	dbHash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	appHash, err := serviceHashWithResolvedRefs(app, nil)
	assert.NilError(t, err)
	consumerHash, err := serviceHashWithResolvedRefs(consumer, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{
		observedServiceContainer("db", 1, container.StateRunning, dbHash),
	}
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StatePaused, appHash),
	}
	observed.Containers["consumer"] = []ObservedContainer{
		observedServiceContainer("consumer", 1, container.StateExited, consumerHash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var appRestartNode, consumerStartNode *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:app:1":
			appRestartNode = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:consumer:1":
			consumerStartNode = n
		}
	}
	if appRestartNode == nil {
		t.Fatalf("expected the create-phase bare-restart node for app:\n%s", plan)
	}
	if consumerStartNode == nil {
		t.Fatalf("expected a start node for consumer:\n%s", plan)
	}
	assert.Assert(t, slices.Contains(consumerStartNode.DependsOn, appRestartNode),
		"consumer's start must depend on app's own restart node, not just its depends_on wait:\n%s", plan)
}

// TestPlanStart_ScaledExceptionalStateConsumerWaitsForEveryRestart is a
// regression test for a Copilot review finding on
// TestPlanStart_ExceptionalStateConsumerWaitsForRestart's own fix: for a
// scaled service, serviceNodes keeps only the LAST create-phase node
// processed, so a consumer's start only ended up depending on the highest-
// numbered replica's restart. With two exceptional-state replicas, the
// consumer must depend on BOTH restart nodes, not just one.
func TestPlanStart_ScaledExceptionalStateConsumerWaitsForEveryRestart(t *testing.T) {
	two := 2
	app := types.ServiceConfig{Name: "app", Deploy: &types.DeployConfig{Replicas: &two}}
	consumer := serviceWithDeps("consumer", types.DependsOnConfig{"app": {Condition: types.ServiceConditionStarted, Required: true}})
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"app": app, "consumer": consumer},
	}
	appHash, err := serviceHashWithResolvedRefs(app, nil)
	assert.NilError(t, err)
	consumerHash, err := serviceHashWithResolvedRefs(consumer, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["app"] = []ObservedContainer{
		observedServiceContainer("app", 1, container.StatePaused, appHash),
		observedServiceContainer("app", 2, container.StateDead, appHash),
	}
	observed.Containers["consumer"] = []ObservedContainer{
		observedServiceContainer("consumer", 1, container.StateExited, consumerHash),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeCreateStart), noPrompt)
	assert.NilError(t, err)

	var restart1, restart2, consumerStartNode *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:app:1":
			restart1 = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:app:2":
			restart2 = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:consumer:1":
			consumerStartNode = n
		}
	}
	if restart1 == nil || restart2 == nil {
		t.Fatalf("expected both replicas' bare-restart nodes:\n%s", plan)
	}
	if consumerStartNode == nil {
		t.Fatalf("expected a start node for consumer:\n%s", plan)
	}
	assert.Assert(t, slices.Contains(consumerStartNode.DependsOn, restart1),
		"consumer's start must depend on replica 1's restart node:\n%s", plan)
	assert.Assert(t, slices.Contains(consumerStartNode.DependsOn, restart2),
		"consumer's start must depend on replica 2's restart node too, not just the last one processed:\n%s", plan)
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
	web := serviceWithDeps("web", types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionStarted, Required: true},
	})
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

// TestPlanStart_ProviderRelayRestart is a regression test for a real bug
// found by the e2e suite (TestProviderPublishEndpoint) once ScopeStart
// became the real, exercised path for `compose start`: planProviderStart
// only ever ordered a provider after its own create-phase OpRunProvider node
// (serviceNodes[service.Name]) -- never set under pure ScopeStart, since the
// create phase that emits it doesn't run at all. A stopped relay container
// therefore had no start node planned for it anywhere, and `compose start`
// on a provider-backed service returned success without actually starting
// it. planProviderRelayRestart now emits a bare OpStartContainer for an
// observed, non-running relay container when the create phase hasn't
// already covered it.
func TestPlanStart_ProviderRelayRestart(t *testing.T) {
	prov := types.ServiceConfig{Name: "prov"}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	observed := emptyObserved()
	relay := observedServiceContainer("prov", 1, container.StateExited, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	observed.Containers["prov"] = []ObservedContainer{relay}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:prov:1, StartContainer, start [start:prov:1] {start}
`)+"\n")
}

// A running relay needs no start node at all: planProviderStart's
// serviceNodes fallback and planProviderRelayRestart both find nothing to
// do, so the provider contributes no start-phase node of its own.
func TestPlanStart_ProviderRelayAlreadyRunningPlansNothing(t *testing.T) {
	prov := types.ServiceConfig{Name: "prov"}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	observed := emptyObserved()
	relay := observedServiceContainer("prov", 1, container.StateRunning, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	observed.Containers["prov"] = []ObservedContainer{relay}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)
	assert.Equal(t, len(plan.Nodes), 0)
}

// TestPlanStart_ProviderRelayRestartRespectsSkipProviders is a regression
// test for a Copilot review finding on planProviderRelayRestart above:
// serviceNodes[service.Name] being absent does not ALWAYS mean "the create
// phase never ran" -- under ScopeCreateStart with SkipProviders (watch's
// rebuild), reconcileService deliberately skips the provider and never sets
// serviceNodes either, an explicit request to leave it alone, not an
// invitation for the start phase to restart its relay regardless.
func TestPlanStart_ProviderRelayRestartRespectsSkipProviders(t *testing.T) {
	prov := types.ServiceConfig{Name: "prov"}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	observed := emptyObserved()
	relay := observedServiceContainer("prov", 1, container.StateExited, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	observed.Containers["prov"] = []ObservedContainer{relay}

	options := startScopeOptions(ScopeCreateStart)
	options.SkipProviders = true
	plan, err := reconcile(t.Context(), project, observed, options, noPrompt)
	assert.NilError(t, err)
	assert.Equal(t, len(plan.Nodes), 0, "SkipProviders must leave a stopped relay untouched:\n%s", plan)
}

// TestPlanStart_ProviderRelayRestartWaitsOnDependencies is a regression test
// for a Copilot review finding on planProviderRelayRestart: under pure
// ScopeStart there is no Create->Start barrier separating a provider from
// its siblings (unlike ScopeCreateStart's OpRunProvider, always a
// create-phase node run to completion before any start-phase node begins).
// Without its own depends_on wired into the relay-restart node, a stopped
// relay could start before a required dependency's health condition
// resolved.
func TestPlanStart_ProviderRelayRestartWaitsOnDependencies(t *testing.T) {
	db := types.ServiceConfig{Name: "db"}
	db.HealthCheck = &types.HealthCheckConfig{Test: []string{"CMD", "true"}}
	prov := serviceWithDeps("prov", types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionHealthy, Required: true},
	})
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"db": db, "prov": prov},
	}
	dbHash, err := serviceHashWithResolvedRefs(db, nil)
	assert.NilError(t, err)
	observed := emptyObserved()
	observed.Containers["db"] = []ObservedContainer{observedServiceContainer("db", 1, container.StateRunning, dbHash)}
	relay := observedServiceContainer("prov", 1, container.StateExited, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	observed.Containers["prov"] = []ObservedContainer{relay}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	var wait, restart *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpWaitCondition:
			wait = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:prov:1":
			restart = n
		}
	}
	if wait == nil || restart == nil {
		t.Fatalf("expected db's wait node and prov's relay restart node:\n%s", plan)
	}
	assert.Assert(t, slices.Contains(restart.DependsOn, wait),
		"the relay restart must depend on prov's own depends_on wait, not start unconditionally:\n%s", plan)
}

// TestPlanStart_OrdinaryPathBareStartsAStaleRelay is a regression test for a
// Copilot review finding on #14296: relay detection must apply per observed
// container, not per the service's CURRENTLY declared type. A service whose
// provider: declaration was removed without an intervening `up`/create to
// converge the daemon still has its old relay container observed under its
// name; planServiceStart's ordinary (non-provider) path must still give it a
// bare start (no secret/config injection, no post_start hooks) -- enriching
// it would act on a shell-less scratch binary with no process to inject into
// or exec hooks against.
func TestPlanStart_OrdinaryPathBareStartsAStaleRelay(t *testing.T) {
	web := types.ServiceConfig{
		Name:      "web",
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"notify"}}},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": web},
	}
	relay := observedServiceContainer("web", 1, container.StateExited, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	observed := emptyObserved()
	observed.Containers["web"] = []ObservedContainer{relay}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	var start *PlanNode
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:web:1" {
			start = n
		}
		assert.Assert(t, n.Operation.Type != OpRunPostStart,
			"post_start must not run against a stale relay container:\n%s", plan)
	}
	if start == nil {
		t.Fatalf("expected a start node for web's stale relay:\n%s", plan)
	}
	assert.Assert(t, start.Operation.Service == nil,
		"a stale relay's start must stay bare (no injection) -- it's not a real service container:\n%s", plan)
}

// TestPlanStart_ProviderPathEnrichesStaleNonRelayContainer is a regression
// test for the second Copilot review finding on #14296, the symmetric gap:
// a service declared provider: whose observed container is NOT a relay (a
// stale normal replica left over from before the service became
// provider-backed) must still be started -- the OLD imperative engine's
// startService only special-cased Provider != nil for the ZERO-containers
// case, so an existing non-relay container was always started normally,
// including post_start hooks. planProviderRelayRestart's first version
// silently dropped this case (filtered on isRelayContainer), returning
// success without starting anything.
func TestPlanStart_ProviderPathEnrichesStaleNonRelayContainer(t *testing.T) {
	prov := types.ServiceConfig{
		Name:      "prov",
		PreStart:  []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"notify"}}},
	}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	stale := observedServiceContainer("prov", 1, container.StateExited, "")
	observed := emptyObserved()
	observed.Containers["prov"] = []ObservedContainer{stale}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	var pre, start, post *PlanNode
	for _, n := range plan.Nodes {
		switch {
		case n.Operation.Type == OpRunPreStart:
			pre = n
		case n.Operation.Type == OpStartContainer && n.Operation.ResourceID == "service:prov:1":
			start = n
		case n.Operation.Type == OpRunPostStart:
			post = n
		}
	}
	if start == nil {
		t.Fatalf("expected a start node for prov's stale non-relay container, not silently skipped:\n%s", plan)
	}
	assert.Assert(t, start.Operation.Service != nil,
		"a stale non-relay container must get an enriched start (injection applies), like any ordinary replica:\n%s", plan)
	// A Copilot review finding on the post_start-only version of this fix:
	// pre_start must also run for this path, exactly like the imperative
	// engine's own lowestNumberedContainer(toStart) + isRelayContainer gate.
	if pre == nil {
		t.Fatalf("expected pre_start hooks to run before starting prov's stale non-relay container:\n%s", plan)
	}
	if post == nil {
		t.Fatalf("expected post_start hooks to run for the enriched start:\n%s", plan)
	}
}

// TestPlanStart_ProviderPathChainsMultipleStaleContainers is a regression
// test for a docker-agent review finding (100/100 confidence) on
// planProviderRelayRestart: the relay is uniquely named, so at most one can
// exist, but a service transitioning to provider-backed without an
// intervening reconciliation can leave several stale non-relay replicas
// behind (e.g. a scale>1 ordinary service whose provider: declaration was
// just added). The first version of this function returned after the first
// not-running container, silently dropping every other one.
func TestPlanStart_ProviderPathChainsMultipleStaleContainers(t *testing.T) {
	prov := types.ServiceConfig{Name: "prov"}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	observed := emptyObserved()
	// Listed out of order on purpose (daemon list order, not replica-number
	// order) -- a second Copilot review finding on this same function: it
	// iterated observed.Containers directly, so the chain could end up
	// ordered by daemon list order instead of replica number, unlike
	// startPhaseReplicas' own sort. The chain below must still end up 1->2.
	observed.Containers["prov"] = []ObservedContainer{
		observedServiceContainer("prov", 2, container.StateExited, ""),
		observedServiceContainer("prov", 1, container.StateExited, ""),
	}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	var start1, start2 *PlanNode
	for _, n := range plan.Nodes {
		switch n.Operation.ResourceID {
		case "service:prov:1":
			start1 = n
		case "service:prov:2":
			start2 = n
		}
	}
	if start1 == nil || start2 == nil {
		t.Fatalf("expected both stale containers to be started, not just the first:\n%s", plan)
	}
	assert.Assert(t, slices.Contains(start2.DependsOn, start1),
		"replica 2 must chain after replica 1's start, in replica-number order regardless of daemon list order:\n%s", plan)
}

// TestPlanStart_ProviderPathPreStartGateDoesNotRetryPastARelay is a
// regression test for a Copilot review finding on
// planProviderRelayRestart's pre_start gate: it used to re-check "is this
// replica a relay?" on every iteration of the chain-building loop, so when
// the lowest-numbered stale container was the relay (skipped) but a later
// one was an ordinary stale replica, pre_start still ran against that later
// one. The old imperative engine's own gate -- lowestNumberedContainer(toStart)
// checked once -- never falls through like that: if its single candidate is
// a relay, pre_start is skipped entirely for the whole chain, with no
// fallback to a later non-relay container.
func TestPlanStart_ProviderPathPreStartGateDoesNotRetryPastARelay(t *testing.T) {
	prov := types.ServiceConfig{
		Name:     "prov",
		PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
	}
	prov.Provider = &types.ServiceProviderConfig{Type: "test"}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"prov": prov},
	}
	relay := observedServiceContainer("prov", 1, container.StateExited, "")
	relay.Summary.Labels[api.RelayLabel] = "relay-abc123"
	stale := observedServiceContainer("prov", 2, container.StateExited, "")
	observed := emptyObserved()
	observed.Containers["prov"] = []ObservedContainer{relay, stale}

	plan, err := reconcile(t.Context(), project, observed, startScopeOptions(ScopeStart), noPrompt)
	assert.NilError(t, err)

	for _, n := range plan.Nodes {
		assert.Assert(t, n.Operation.Type != OpRunPreStart,
			"pre_start must not run when the lowest-numbered stale container is a relay, even though a later one (#2) is not:\n%s", plan)
	}
}

// An optional (required: false) condition marks the shared wait node
// best-effort — a missing dependency is skipped, not fatal; one required
// dependent upgrades the node for everyone.
func TestPlanStart_OptionalConditionIsBestEffort(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db"},
			"web": serviceWithDeps("web", types.DependsOnConfig{
				"db": {Condition: types.ServiceConditionHealthy, Required: false},
			}),
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
	project.Services["worker"] = serviceWithDeps("worker", types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionHealthy, Required: true},
	})
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
