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
	"net"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// expectHook expects one hook to be run in the container: its exec, with the
// given exit code. It returns the last call of the sequence, for ordering.
func expectHook(apiClient *mocks.MockAPIClient, ctr container.Summary, execID string, exitCode int) *gomock.Call {
	create := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{ID: execID}, nil)
	server, conn := net.Pipe()
	_ = server.Close()
	attach := apiClient.EXPECT().ExecAttach(gomock.Any(), execID, gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(conn, "")}, nil).After(create)
	return apiClient.EXPECT().ExecInspect(gomock.Any(), execID, gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: exitCode}, nil).After(attach)
}

// preStopPlan is what the reconciler plans to stop a container whose service
// has pre_stop hooks and to remove it: the hooks, the stop that follows them,
// the removal that follows the stop.
func preStopPlan(ctr *container.Summary, service *types.ServiceConfig, group string) *planning.Plan {
	plan := &planning.Plan{}
	hooks := plan.AddNode(planning.Operation{
		Type: planning.OpRunPreStop, ResourceID: "service:web:1", Cause: "pre_stop hooks", Service: service, Container: ctr,
	}, group)
	stop := plan.AddNode(planning.Operation{
		Type: planning.OpStopContainer, ResourceID: "service:web:1", Cause: "scale down", Container: ctr,
	}, group, hooks)
	plan.AddNode(planning.Operation{
		Type: planning.OpRemoveContainer, ResourceID: "service:web:1", Cause: "scale down", Container: ctr,
	}, group, stop)
	return plan
}

func hookedWeb(hooks ...string) *types.ServiceConfig {
	service := &types.ServiceConfig{Name: "web"}
	for _, h := range hooks {
		service.PreStop = append(service.PreStop, types.ServiceHook{Command: types.ShellCommand{h}})
	}
	return service
}

func TestExecutePlanRunsPreStopHooksBeforeStoppingTheContainer(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)
	ctr := serviceContainer("web")
	service := hookedWeb("quiesce", "flush")

	flushed := expectHook(apiClient, ctr, "exec-1", 0)
	flushed = expectHook(apiClient, ctr, "exec-2", 0).After(flushed) // the second hook runs after the first
	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, nil).After(flushed)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil).After(stopped)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, service, ""))
	assert.NilError(t, err)

	// the hooks have no row of their own to report on: the container's row goes
	// from stopping to removed, as it did without hooks
	assert.DeepEqual(t, rec.summary(), []string{
		"Container prj-web-1: Stopping",
		"Container prj-web-1: Stopped",
		"Container prj-web-1: Removing",
		"Container prj-web-1: Removed",
	})
}

// Hooks of a recreation belong to its group: the recreation is one row,
// working at its first node and done at its last, the hooks included.
func TestExecutePlanKeepsTheHooksOfARecreationInItsGroup(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)
	ctr := serviceContainer("web")

	hooks := expectHook(apiClient, ctr, "exec-1", 0)
	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, nil).After(hooks)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil).After(stopped)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("quiesce"), "recreate:web:1"))
	assert.NilError(t, err)

	assert.DeepEqual(t, rec.summary(), []string{
		"Container prj-web-1: Recreate",
		"Container prj-web-1: Recreated",
	})
}

// A container with no process to run the hooks in (stopped, paused) answers the
// exec with a conflict: the hooks are skipped and the stop goes on.
func TestExecutePlanStopsAContainerWhoseHooksHaveNoProcessToRunIn(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	ctr := serviceContainer("web")
	ctr.State = container.StatePaused

	hooks := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{}, errdefs.ErrConflict.WithMessage("container is paused"))
	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, nil).After(hooks)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil).After(stopped)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("quiesce", "flush"), ""))
	assert.NilError(t, err)
}

// A hook that fails fails the plan, and the container is neither stopped nor
// removed: the hook was to prepare a stop that does not happen. No ContainerStop
// nor ContainerRemove expectation is set: per newStartTestService, gomock fails
// the test if either still runs.
func TestExecutePlanDoesNotStopAContainerWhoseHookFails(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)
	ctr := serviceContainer("web")

	expectHook(apiClient, ctr, "exec-1", 3)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("quiesce", "flush"), ""))
	assert.ErrorContains(t, err, "web hook exited with status 3")

	var reported bool
	for _, e := range rec.events {
		if e.ID == "Container prj-web-1" && e.Status == api.Error {
			reported = true
			assert.Assert(t, e.Text != "")
		}
	}
	assert.Assert(t, reported, "the failure is reported on the row of the container: %v", rec.summary())
	// the row was stopping while the hooks ran, and never got to stopped
	assert.Assert(t, rec.contains("Container prj-web-1: Stopping"), "%v", rec.summary())
	assert.Assert(t, !rec.contains("Container prj-web-1: Stopped"), "%v", rec.summary())
}

// The row of a container is stopping while its hooks run, once: the stop that
// follows them does not say it again, and a stop without hooks says it as ever.
func TestStoppingIsAnnouncedOnceWhetherOrNotHooksRunFirst(t *testing.T) {
	ctr := serviceContainer("web")
	withHooks := preStopPlan(&ctr, hookedWeb("quiesce"), "")
	withoutHooks := &planning.Plan{}
	stop := withoutHooks.AddNode(planning.Operation{Type: planning.OpStopContainer, ResourceID: "service:web:1", Container: &ctr}, "")

	for name, tc := range map[string]struct {
		node *planning.PlanNode
		want []string
	}{
		"the hooks":             {withHooks.Nodes[0], []string{"Container prj-web-1: Stopping"}},
		"the stop that follows": {withHooks.Nodes[1], nil},
		"a stop without hooks":  {stop, []string{"Container prj-web-1: Stopping"}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recordingEventProcessor{}
			emitStartEvent(tc.node, rec)
			assert.DeepEqual(t, rec.summary(), append([]string{}, tc.want...))
		})
	}
	// and nothing is said when the hooks end: the stop says when the row is stopped
	rec := &recordingEventProcessor{}
	emitDoneEvent(withHooks.Nodes[0], rec)
	assert.Equal(t, len(rec.summary()), 0)
}

// The hooks that follow a failing one do not run.
func TestExecutePlanStopsRunningHooksAtTheFirstFailure(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	ctr := serviceContainer("web")

	expectHook(apiClient, ctr, "exec-1", 1) // the second hook has no expectation: it must not run

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("first", "second"), ""))
	assert.ErrorContains(t, err, "exited with status 1")
}

// A container removed since it was listed: the hook finds nothing and the stop
// reports it, as it does for a container without hooks.
func TestExecutePlanLeavesAGoneContainerToTheStop(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	ctr := serviceContainer("web")

	hooks := apiClient.EXPECT().ExecCreate(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ExecCreateResult{}, errdefs.ErrNotFound.WithMessage("No such container"))
	apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, errdefs.ErrNotFound.WithMessage("No such container")).After(hooks)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("quiesce"), ""))
	assert.Assert(t, errdefs.IsNotFound(err), "%v", err)
}

// A dry run has no side effect: the hooks are not run, and the dry-run client
// could not attach to their exec. No ExecCreate expectation is set: per
// newStartTestService, gomock fails the test if a hook still runs.
func TestExecutePlanSkipsPreStopHooksInDryRun(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	svc.dryRun = true
	ctr := serviceContainer("web")

	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), ctr.ID, gomock.Any()).Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), ctr.ID, gomock.Any()).Return(client.ContainerRemoveResult{}, nil).After(stopped)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&ctr, hookedWeb("quiesce"), ""))
	assert.NilError(t, err)
}

// A relay has no process for a hook to act on, even if a plan names one.
func TestExecutePlanSkipsPreStopHooksOfARelay(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)
	relay := serviceContainer("web")
	relay.Labels[api.RelayLabel] = "abc123"

	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), relay.ID, gomock.Any()).Return(client.ContainerStopResult{}, nil)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), relay.ID, gomock.Any()).Return(client.ContainerRemoveResult{}, nil).After(stopped)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), preStopPlan(&relay, hookedWeb("quiesce"), ""))
	assert.NilError(t, err)
}

// No hook runs for a plan that is already canceled: nothing is stopped for
// want of the hooks either.
func TestExecRunPreStopDoesNothingOnceCanceled(t *testing.T) {
	svc, _, _ := newStartTestService(t)
	ctr := serviceContainer("web")
	exec := svc.newPlanExecutor(&types.Project{Name: "prj"}, emptyObservedState("prj"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := exec.execRunPreStop(ctx, planning.Operation{Type: planning.OpRunPreStop, Service: hookedWeb("quiesce"), Container: &ctr})
	assert.ErrorIs(t, err, context.Canceled)
}

// From the observed state to the calls on the daemon: what the reconciler plans
// for a scale down is what runs, hooks first, only for the replica that goes.
func TestScaleDownRunsTheHooksOfTheReplicaThatGoesThenStopsAndRemovesIt(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	web := hookedWebService()
	project := &types.Project{Name: "prj", Services: types.Services{"web": web}}
	replicas := runningReplicas(t, web, 2)
	observed := observedOf("prj", map[string][]ObservedContainer{"web": replicas})

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	gone := replicas[1].Summary
	hooks := expectHook(apiClient, gone, "exec-1", 0)
	stopped := apiClient.EXPECT().ContainerStop(gomock.Any(), gone.ID, gomock.Any()).
		Return(client.ContainerStopResult{}, nil).After(hooks)
	apiClient.EXPECT().ContainerRemove(gomock.Any(), gone.ID, gomock.Any()).
		Return(client.ContainerRemoveResult{}, nil).After(stopped)

	assert.NilError(t, svc.executePlan(t.Context(), project, observed, plan))
}

// A hook that fails in a recreation fails it before anything is created: the
// row of the container is recreating, then in error, and the replacement is not
// created. No ContainerCreate expectation is set: per newStartTestService,
// gomock fails the test if it still happens.
func TestExecutePlanDoesNotCreateTheReplacementWhenTheHooksOfARecreationFail(t *testing.T) {
	svc, apiClient, rec := newStartTestService(t)
	ctr := serviceContainer("web")
	service := hookedWeb("quiesce")
	const group = "recreate:web:1"

	plan := &planning.Plan{}
	hooks := plan.AddNode(planning.Operation{
		Type: planning.OpRunPreStop, ResourceID: "service:web:1", Cause: "pre_stop hooks", Service: service, Container: &ctr,
	}, group)
	create := plan.AddNode(planning.Operation{
		Type: planning.OpCreateContainer, ResourceID: "service:web:1", Cause: "config changed (tmpName)", Service: service, Name: "abc_prj-web-1", Number: 1,
	}, group, hooks)
	stop := plan.AddNode(planning.Operation{Type: planning.OpStopContainer, ResourceID: "service:web:1", Container: &ctr}, group, create)
	plan.AddNode(planning.Operation{Type: planning.OpRemoveContainer, ResourceID: "service:web:1", Container: &ctr}, group, stop)

	expectHook(apiClient, ctr, "exec-1", 3)

	err := svc.executePlan(t.Context(), &types.Project{Name: "prj"}, emptyObservedState("prj"), plan)
	assert.ErrorContains(t, err, "web hook exited with status 3")

	events := rec.summary()
	assert.Equal(t, len(events), 2, "%v", events)
	assert.Equal(t, events[0], "Container prj-web-1: Recreate")
	assert.Assert(t, strings.HasPrefix(events[1], "Container prj-web-1: web hook exited with status 3"), "%v", events)
	assert.Equal(t, rec.events[1].Status, api.Error)
}
