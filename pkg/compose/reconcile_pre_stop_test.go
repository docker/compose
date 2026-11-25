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

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/pkg/api"
)

// Whichever operation of the plan decides to stop a container, the pre_stop
// hooks of its service run first, as a node of their own that the stop depends
// on. Before, only `restart` and `down` ran them: an `up` that recreated a
// container, or scaled a service down, did not.

var quiesceHook = []types.ServiceHook{{Command: types.ShellCommand{"quiesce"}}}

// hookedWebService is the service "web", with a pre_stop hook.
func hookedWebService() types.ServiceConfig {
	return types.ServiceConfig{Name: "web", Scale: intPtr(1), PreStop: quiesceHook}
}

// runningReplicas observes the replicas 1..n of svc, running and up to date.
func runningReplicas(t *testing.T, svc types.ServiceConfig, n int) []ObservedContainer {
	t.Helper()
	hash := mustServiceHash(t, svc)
	var replicas []ObservedContainer
	for i := 1; i <= n; i++ {
		id := svc.Name + "-" + strconv.Itoa(i)
		replicas = append(replicas, ObservedContainer{
			ID: id, Number: i, State: container.StateRunning, ConfigHash: hash,
			Summary: container.Summary{
				ID: id, Names: []string{"/prj-" + svc.Name + "-" + strconv.Itoa(i)}, State: container.StateRunning,
				Labels: map[string]string{api.ServiceLabel: svc.Name, api.ContainerNumberLabel: strconv.Itoa(i), api.ConfigHashLabel: hash},
			},
		})
	}
	return replicas
}

func observedOf(project string, containers map[string][]ObservedContainer) *ObservedState {
	return &ObservedState{
		ProjectName: project,
		Containers:  containers,
		Networks:    map[string][]ObservedNetwork{},
		Volumes:     map[string][]ObservedVolume{},
	}
}

// nodesOf lists the nodes of a type, in plan order.
func nodesOf(plan *planning.Plan, t planning.OperationType) []*planning.PlanNode {
	var nodes []*planning.PlanNode
	for _, node := range plan.Nodes {
		if node.Operation.Type == t {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// hooksBeforeStop checks that the container is stopped once, and that this stop
// has the hooks of the container as its only dependency: whatever the stop was
// waiting for, the hooks wait for. It returns the two nodes.
func hooksBeforeStop(t *testing.T, plan *planning.Plan, containerID string) (hooks, stop *planning.PlanNode) {
	t.Helper()
	for _, node := range nodesOf(plan, planning.OpStopContainer) {
		if node.Operation.Container.ID == containerID {
			assert.Assert(t, stop == nil, "container %s is stopped twice:\n%s", containerID, plan)
			stop = node
		}
	}
	assert.Assert(t, stop != nil, "container %s is not stopped:\n%s", containerID, plan)
	assert.Equal(t, len(stop.DependsOn), 1, "the stop of %s waits for its hooks and for them only (they wait for the rest):\n%s", containerID, plan)
	hooks = stop.DependsOn[0]
	assert.Equal(t, hooks.Operation.Type, planning.OpRunPreStop, "the stop of %s does not follow its hooks:\n%s", containerID, plan)
	assert.Equal(t, hooks.Operation.Container.ID, containerID)
	assert.Assert(t, hooks.Operation.Service != nil && len(hooks.Operation.Service.PreStop) > 0, "the hooks node carries the hooks to run")
	return hooks, stop
}

func TestReconcileRunsPreStopHooksBeforeAScaleDown(t *testing.T) {
	svc := hookedWebService()
	project := &types.Project{Name: "myproject", Services: types.Services{"web": svc}}
	observed := observedOf("myproject", map[string][]ObservedContainer{"web": runningReplicas(t, svc, 2)})

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:2, RunPreStop, pre_stop hooks
[1] -> #2 service:web:2, StopContainer, scale down
[2] -> #3 service:web:2, RemoveContainer, scale down
`)+"\n")
	hooksBeforeStop(t, plan, "web-2")
	assert.Equal(t, len(nodesOf(plan, planning.OpRunPreStop)), 1, "only the replica that goes has hooks to run:\n%s", plan)
}

func TestReconcileRunsPreStopHooksOfTheContainerItReplacesBeforeCreatingTheReplacement(t *testing.T) {
	svc := hookedWebService()
	project := &types.Project{Name: "myproject", Services: types.Services{"web": svc}}
	replicas := runningReplicas(t, svc, 1)
	replicas[0].ConfigHash = "stale"
	replicas[0].Summary.Labels[api.ConfigHashLabel] = "stale"
	observed := observedOf("myproject", map[string][]ObservedContainer{"web": replicas})

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	// the hooks come first: one that fails leaves no replacement, under its
	// temporary name, for the next up to trip on. Then the replacement is created,
	// and only then is the old container stopped and removed.
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, RunPreStop, pre_stop hooks [recreate:web:1]
[1] -> #2 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[2] -> #3 service:web:1, StopContainer, replaced by #2 [recreate:web:1]
[3] -> #4 service:web:1, RemoveContainer, replaced by #2 [recreate:web:1]
[4] -> #5 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")

	hooks := nodesOf(plan, planning.OpRunPreStop)[0]
	create := nodesOf(plan, planning.OpCreateContainer)[0]
	stop := nodesOf(plan, planning.OpStopContainer)[0]
	assert.DeepEqual(t, create.DependsOn, []*planning.PlanNode{hooks})
	assert.DeepEqual(t, stop.DependsOn, []*planning.PlanNode{create})
	assert.Equal(t, hooks.Operation.Container.ID, "web-1")
	assert.Equal(t, hooks.Group, "recreate:web:1", "the hooks belong to the recreation, which reports as one row")
}

func TestReconcileRecreatesWithoutHooksAsItAlwaysDid(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
	project := &types.Project{Name: "myproject", Services: types.Services{"web": svc}}
	replicas := runningReplicas(t, svc, 1)
	replicas[0].ConfigHash = "stale"
	replicas[0].Summary.Labels[api.ConfigHashLabel] = "stale"
	observed := observedOf("myproject", map[string][]ObservedContainer{"web": replicas})

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[1] -> #2 service:web:1, StopContainer, replaced by #1 [recreate:web:1]
[2] -> #3 service:web:1, RemoveContainer, replaced by #1 [recreate:web:1]
[3] -> #4 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")
}

func TestReconcileRunsPreStopHooksOfTheContainersOfARecreatedNetwork(t *testing.T) {
	web := types.ServiceConfig{
		Name: "web", Scale: intPtr(1), PreStop: quiesceHook,
		ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}}},
	}
	project := &types.Project{
		Name:     "myproject",
		Networks: types.Networks{"frontend": {Name: "myproject_frontend", Driver: "overlay"}},
		Services: types.Services{"web": web},
	}
	observed := observedOf("myproject", map[string][]ObservedContainer{"web": {networkAttachedContainer(t, web, "web-1")}})
	observed.Networks["frontend"] = []ObservedNetwork{{ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"}}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	hooks, stop := hooksBeforeStop(t, plan, "web-1")
	assert.Equal(t, hooks.Operation.Cause, "pre_stop hooks")
	// the container is disconnected once it is stopped, not before
	disconnect := nodesOf(plan, planning.OpDisconnectNetwork)
	assert.Equal(t, len(disconnect), 1)
	assert.Equal(t, disconnect[0].DependsOn[0], stop)
}

func TestReconcileRunsPreStopHooksOfTheContainersOfARecreatedVolume(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 2)
	svc := project.Services["db0"]
	svc.PreStop = quiesceHook
	project.Services["db0"] = svc
	// the hooks are part of what the containers were created from
	for i, oc := range observed.Containers["db0"] {
		hash := mustServiceHash(t, svc)
		observed.Containers["db0"][i].ConfigHash = hash
		oc.Summary.Labels[api.ConfigHashLabel] = hash
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	for _, id := range []string{"db0-1", "db0-2"} {
		_, stop := hooksBeforeStop(t, plan, id)
		assert.Equal(t, stop.Operation.Cause, "mounted volume config changed")
	}
	assert.Equal(t, len(nodesOf(plan, planning.OpRunPreStop)), 2)
}

// A dependent that restarts with its dependency is stopped, with its hooks,
// when the dependency is recreated.
func TestReconcileRunsPreStopHooksOfTheDependentsOfARecreatedService(t *testing.T) {
	parent := types.ServiceConfig{Name: "parent", Scale: intPtr(1), ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	dependent := types.ServiceConfig{
		Name: "dependent", Scale: intPtr(1), PreStop: quiesceHook,
		ContainerSpec: types.ContainerSpec{Image: "alpine"},
		WorkloadSpec: types.WorkloadSpec{
			DependsOn: types.DependsOnConfig{"parent": {Condition: types.ServiceConditionStarted, Restart: true, Required: true}},
		},
	}
	project := &types.Project{Name: "myproject", Services: types.Services{"parent": parent, "dependent": dependent}}
	observed := parentDependentObserved(t, parent, dependent)
	observed.Containers["parent"][0].ConfigHash = "stale_parent_hash"
	observed.Containers["parent"][0].Summary.Labels[api.ConfigHashLabel] = "stale_parent_hash"

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	hooksBeforeStop(t, plan, "dependent_container_xyz")
	// stopped for the cascade and replaced with it: one stop, so one run of hooks
	assert.Equal(t, len(nodesOf(plan, planning.OpRunPreStop)), 1, "\n%s", plan)
}

// A container that two decisions of the plan want stopped is stopped once, and
// its hooks run once: here a recreated network and a changed service.
func TestReconcileRunsPreStopHooksOnceForAContainerStoppedForTwoReasons(t *testing.T) {
	web := types.ServiceConfig{
		Name: "web", Scale: intPtr(1), PreStop: quiesceHook,
		ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}}},
	}
	project := &types.Project{
		Name:     "myproject",
		Networks: types.Networks{"frontend": {Name: "myproject_frontend", Driver: "overlay"}},
		Services: types.Services{"web": web},
	}
	oc := networkAttachedContainer(t, web, "web-1")
	oc.ConfigHash = "stale"
	oc.Summary.Labels[api.ConfigHashLabel] = "stale"
	observed := observedOf("myproject", map[string][]ObservedContainer{"web": {oc}})
	observed.Networks["frontend"] = []ObservedNetwork{{ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"}}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, len(nodesOf(plan, planning.OpStopContainer)), 1, "\n%s", plan)
	assert.Equal(t, len(nodesOf(plan, planning.OpRunPreStop)), 1, "\n%s", plan)
	_, stop := hooksBeforeStop(t, plan, "web-1")

	// the stop was scheduled by the network, ahead of the recreation: the
	// replacement is not created before its hooks have run, so that a hook that
	// fails leaves none behind, and the old container is still not removed before
	// the replacement exists
	hooks, _ := hooksBeforeStop(t, plan, "web-1")
	create := nodesOf(plan, planning.OpCreateContainer)[0]
	assert.Assert(t, slices.Contains(create.DependsOn, hooks), "the replacement is created while the hooks may still fail:\n%s", plan)
	remove := nodesOf(plan, planning.OpRemoveContainer)[0]
	assert.Assert(t, slices.Contains(remove.DependsOn, create), "the removal does not wait for the replacement:\n%s", plan)
	assert.Assert(t, slices.Contains(remove.DependsOn, stop), "the removal does not wait for the stop:\n%s", plan)
}

// No hooks node where there is no hook to run: the plans of services without
// pre_stop hooks are the ones they always were.
func TestReconcileAddsNoPreStopNodeWithoutHooksToRun(t *testing.T) {
	t.Run("a service without hooks", func(t *testing.T) {
		svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
		project := &types.Project{Name: "myproject", Services: types.Services{"web": svc}}
		observed := observedOf("myproject", map[string][]ObservedContainer{"web": runningReplicas(t, svc, 2)})

		plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
		assert.NilError(t, err)
		assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:2, StopContainer, scale down
[1] -> #2 service:web:2, RemoveContainer, scale down
`)+"\n")
	})

	t.Run("an orphan, which is not in the model and has no hooks to know", func(t *testing.T) {
		project := &types.Project{Name: "myproject", Services: types.Services{}}
		observed := observedOf("myproject", map[string][]ObservedContainer{})
		observed.Orphans = []ObservedContainer{{
			ID: "orphan1", Number: 1, Name: "myproject-old-1",
			Summary: container.Summary{ID: "orphan1", Labels: map[string]string{api.ServiceLabel: "old"}},
		}}
		opts := defaultReconcileOptions()
		opts.RemoveOrphans = true

		plan, err := reconcile(t.Context(), project, observed, opts, noPrompt)
		assert.NilError(t, err)
		assert.Equal(t, len(nodesOf(plan, planning.OpRunPreStop)), 0, "\n%s", plan)
		assert.Equal(t, len(nodesOf(plan, planning.OpStopContainer)), 1)
	})
}

func TestServiceWithPreStopHooks(t *testing.T) {
	web := hookedWebService()
	plain := types.ServiceConfig{Name: "plain", Scale: intPtr(1)}
	r := &reconciler{project: &types.Project{Services: types.Services{"web": web, "plain": plain}}}
	ctr := func(service string, labels ...string) *container.Summary {
		c := &container.Summary{Labels: map[string]string{api.ServiceLabel: service}}
		for i := 0; i+1 < len(labels); i += 2 {
			c.Labels[labels[i]] = labels[i+1]
		}
		return c
	}

	service, ok := r.serviceWithPreStopHooks(ctr("web"))
	assert.Assert(t, ok)
	assert.Equal(t, service.Name, "web")
	assert.Equal(t, len(service.PreStop), 1)

	for name, c := range map[string]*container.Summary{
		"no container":                                   nil,
		"a service without hooks":                        ctr("plain"),
		"a service that is not in the model":             ctr("gone"),
		"a relay, which has no process to run a hook in": ctr("web", api.RelayLabel, "abc123"),
	} {
		_, ok := r.serviceWithPreStopHooks(c)
		assert.Assert(t, !ok, name)
	}
}

// A service that used to run containers and is now served by a provider: the
// replicas it leaves behind are stopped, with their hooks, before the provider
// takes their name.
func TestReconcileRunsPreStopHooksOfTheReplicasAServiceLeavesForAProvider(t *testing.T) {
	db := types.ServiceConfig{Name: "db", PreStop: quiesceHook, Provider: &types.ServiceProviderConfig{Type: "test"}}
	project := &types.Project{Name: "myproject", Services: types.Services{"db": db}}
	stale := runningReplicas(t, types.ServiceConfig{Name: "db"}, 1)
	observed := observedOf("myproject", map[string][]ObservedContainer{"db": stale})

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, RunPreStop, pre_stop hooks
[1] -> #2 service:db:1, StopContainer, service is now provider-backed
[2] -> #3 service:db:1, RemoveContainer, service is now provider-backed
[3] -> #4 provider:db, RunProvider, provider service
`)+"\n")
	hooksBeforeStop(t, plan, "db-1")
}

// The hooks of a recreation wait for what the creation waited for, which
// includes the stop of the dependents the recreation restarts.
func TestReconcileRunsPreStopHooksOfARecreatedServiceAfterTheStopOfItsDependents(t *testing.T) {
	parent := types.ServiceConfig{Name: "parent", Scale: intPtr(1), PreStop: quiesceHook, ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	dependent := types.ServiceConfig{
		Name: "dependent", Scale: intPtr(1), ContainerSpec: types.ContainerSpec{Image: "alpine"},
		WorkloadSpec: types.WorkloadSpec{
			DependsOn: types.DependsOnConfig{"parent": {Condition: types.ServiceConditionStarted, Restart: true, Required: true}},
		},
	}
	project := &types.Project{Name: "myproject", Services: types.Services{"parent": parent, "dependent": dependent}}
	observed := parentDependentObserved(t, parent, dependent)
	observed.Containers["parent"][0].ConfigHash = "stale_parent_hash"
	observed.Containers["parent"][0].Summary.Labels[api.ConfigHashLabel] = "stale_parent_hash"

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	var dependentStop *planning.PlanNode
	for _, node := range nodesOf(plan, planning.OpStopContainer) {
		if node.Operation.Container.ID == "dependent_container_xyz" {
			dependentStop = node
		}
	}
	assert.Assert(t, dependentStop != nil, "\n%s", plan)
	hooks := nodesOf(plan, planning.OpRunPreStop)
	assert.Equal(t, len(hooks), 1, "only the parent has hooks:\n%s", plan)
	assert.Assert(t, slices.Contains(hooks[0].DependsOn, dependentStop), "the hooks of the parent do not wait for the stop of its dependent:\n%s", plan)
	create := nodesOf(plan, planning.OpCreateContainer)[0]
	assert.DeepEqual(t, create.DependsOn, []*planning.PlanNode{hooks[0]})
}
