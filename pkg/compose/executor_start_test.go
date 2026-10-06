//go:build !windows

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
	"sync"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// recordingEvents captures, per resource ID, the sequence of status texts —
// the per-resource progression the event contract promises (see
// api.EventProcessor).
type recordingEvents struct {
	mu   sync.Mutex
	byID map[string][]string
}

func newRecordingEvents() *recordingEvents { return &recordingEvents{byID: map[string][]string{}} }

func (r *recordingEvents) Start(_ context.Context, _ string) {}
func (r *recordingEvents) Done(_ string, _ bool)             {}
func (r *recordingEvents) On(events ...api.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range events {
		r.byID[e.ID] = append(r.byID[e.ID], e.Text)
	}
}

func newStartPhaseTestService(t *testing.T) (*composeService, *mocks.MockAPIClient, *recordingEvents) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("unix:///var/run/docker.sock").AnyTimes()
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

	recorder := newRecordingEvents()
	svcAny, err := NewComposeService(cli, WithEventProcessor(recorder))
	assert.NilError(t, err)
	return svcAny.(*composeService), apiClient, recorder
}

// TestExecWaitCondition_RequiredMissingDependencyFails verifies that a
// required depends_on condition whose dependency has no live container at
// all is a hard plan failure, exactly like waitDependencies today.
func TestExecWaitCondition_RequiredMissingDependencyFails(t *testing.T) {
	svc, _, _ := newStartPhaseTestService(t)
	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	err := exec.execWaitCondition(t.Context(), Operation{
		Name:      "db",
		Condition: types.ServiceConditionHealthy,
	})
	assert.ErrorContains(t, err, "missing dependency db")
}

// TestExecWaitCondition_OptionalMissingDependencyIsTolerated mirrors
// waitDependencies' handling of an optional (required: false) dependency
// with no live container: a warning, not a plan failure, and the wait
// resolves as satisfied.
func TestExecWaitCondition_OptionalMissingDependencyIsTolerated(t *testing.T) {
	svc, _, recorder := newStartPhaseTestService(t)
	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	err := exec.execWaitCondition(t.Context(), Operation{
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
		BestEffort: true,
	})
	assert.NilError(t, err)
	assert.Equal(t, len(recorder.byID), 0, "no dependency container exists to report an event against")
}

// TestExecWaitCondition_HealthyConditionSatisfied drives execWaitCondition
// against a live dependency container that reports healthy on the first
// probe, reusing the exact same isServiceHealthy/waitDependency primitives
// the imperative engine's waitDependencies uses.
func TestExecWaitCondition_HealthyConditionSatisfied(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	observed := &ObservedState{
		ProjectName: "test",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string][]ObservedNetwork{},
		Volumes:     map[string][]ObservedVolume{},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Healthy},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil)

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, observed, nil)
	exec.containersByService["db"] = Containers{dbSummary}

	err := exec.execWaitCondition(t.Context(), Operation{
		Name:      "db",
		Condition: types.ServiceConditionHealthy,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting, api.StatusHealthy})
}

// TestExecStartContainer_EnrichedResolvesCreateNodeAndStarts verifies that a
// start-phase OpStartContainer (Service set) resolves its target through the
// CreateContainer node it references — the same reconciliationContext
// mechanism OpRenameContainer already uses — rather than requiring an
// observed container.Summary.
func TestExecStartContainer_EnrichedResolvesCreateNodeAndStarts(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	service := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "new-id"}, nil)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:              "new-id",
			Name:            "/test-web-1",
			Config:          &container.Config{},
			NetworkSettings: &container.NetworkSettings{},
		}}, nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	plan := &Plan{}
	create := plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:web:1",
		Cause:      "no existing container",
		Service:    &service,
		Name:       "test-web-1",
		Number:     1,
	}, "")
	plan.addNode(Operation{
		Type:         OpStartContainer,
		ResourceID:   "service:web:1",
		Cause:        "start",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", create)

	err := svc.executePlan(t.Context(), project, emptyObservedState("test"), plan)
	assert.NilError(t, err)
}

// TestExecutePlanRecreateThenStartUsesFinalName is a regression test: a
// recreate's create node is planned under a temporary name
// (planRecreateContainer's "<shortID>_<name>" dance, to avoid colliding with
// the old container still holding the final name), and the final name is
// applied by a separate OpRenameContainer node. The start phase's event
// naming (groupEventName) and resolveContainerID both read
// pctx.get(CreateNodeID) by design (plannedReplica keeps every start-phase
// reference pointed at the create node, not the rename node) -- so
// execRenameContainer must update that same entry in place once it renames
// the container, or the start phase reports progress under the stale
// temporary name forever. Caught via e2e (TestRestartWithDependencies)
// before this node-result update existed.
func TestExecutePlanRecreateThenStartUsesFinalName(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	service := types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: "alpine"}}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}

	const tmpName = "abc123456789_test-web-1"
	const finalName = "test-web-1"

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "new-id"}, nil)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:              "new-id",
			Name:            "/" + tmpName,
			Config:          &container.Config{},
			NetworkSettings: &container.NetworkSettings{},
		}}, nil)
	apiClient.EXPECT().ContainerRename(gomock.Any(), "new-id", client.ContainerRenameOptions{NewName: finalName}).
		Return(client.ContainerRenameResult{}, nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	plan := &Plan{}
	create := plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:web:1",
		Cause:      "config changed (tmpName)",
		Service:    &service,
		Name:       tmpName,
		Number:     1,
	}, "")
	rename := plan.addNode(Operation{
		Type:         OpRenameContainer,
		ResourceID:   "service:web:1",
		Cause:        "finalize recreate",
		Name:         finalName,
		CreateNodeID: create.ID,
	}, "", create)
	plan.addNode(Operation{
		Type:         OpStartContainer,
		ResourceID:   "service:web:1",
		Cause:        "start",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", rename)

	err := svc.executePlan(t.Context(), project, emptyObservedState("test"), plan)
	assert.NilError(t, err)

	// The start group's eventName is resolved lazily from
	// pctx.get(CreateNodeID) (see groupEventName) once the start node
	// actually runs -- by then the rename has already completed, so this
	// must be the post-rename name, not create's own temporary one.
	assert.DeepEqual(t, recorder.byID["Container "+finalName], []string{api.StatusStarting, api.StatusStarted})
}

// TestExecRenameContainer_RefreshesLiveViewName is the sibling regression to
// TestExecutePlanRecreateThenStartUsesFinalName: execCreateContainer
// publishes the new container into containersByService (the live view
// OpWaitCondition and sibling execCreateContainer calls read by service
// name) under its temporary name -- the only one it had at that point.
// Without execRenameContainer refreshing that same entry, a dependent
// waiting on this service's health (depends_on: condition: service_healthy)
// reports Waiting/Healthy under the stale temporary name for the rest of the
// plan's execution, exactly like TestRestartWithDependencies caught in e2e.
func TestExecRenameContainer_RefreshesLiveViewName(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	service := types.ServiceConfig{Name: "web"}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}

	const tmpName = "abc123456789_test-web-1"
	const finalName = "test-web-1"

	apiClient.EXPECT().ContainerRename(gomock.Any(), "new-id", client.ContainerRenameOptions{NewName: finalName}).
		Return(client.ContainerRenameResult{}, nil)

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	exec.pctx.set(1, operationResult{ContainerID: "new-id", ContainerName: tmpName})
	exec.containersByService["web"] = Containers{{ID: "new-id", Names: []string{"/" + tmpName}}}

	node := &PlanNode{ID: 2, Operation: Operation{
		Type:         OpRenameContainer,
		Name:         finalName,
		Service:      &service,
		CreateNodeID: 1,
	}}

	err := exec.execRenameContainer(t.Context(), node)
	assert.NilError(t, err)

	assert.DeepEqual(t, exec.containersByService["web"][0].Names, []string{"/" + finalName})
}

// TestExecRunPreStart_SkipsWhenReplicaAlreadyRunning covers the
// observe-to-execute drift guard: the plan scheduled RunPreStart because no
// replica was running at observation time, but a replica started in the
// meantime — the once-per-service rule says the hooks must not run.
func TestExecRunPreStart_SkipsWhenReplicaAlreadyRunning(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name:     "web",
		PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
	}
	running := container.Summary{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web", api.OneoffLabel: "False"}}
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{running}}, nil)
	// No further daemon call: the hook runner scan (and any wait/start/remove)
	// never happens once the drift guard skips the node.

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	err := exec.execRunPreStart(t.Context(), Operation{Service: &service})
	assert.NilError(t, err)
}

// TestExecRunPreStart_BailsOutWhenContextCanceled pins the same interim
// narrowing execStartContainer and execRunPostStart carry: once ctx is
// canceled (a sibling node failed), the node must neither touch the daemon
// nor run hooks. No ContainerList expectation is registered: an unexpected
// call fails the test.
func TestExecRunPreStart_BailsOutWhenContextCanceled(t *testing.T) {
	svc, _, _ := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name:     "web",
		PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	err := exec.execRunPreStart(ctx, Operation{Service: &service})
	assert.ErrorIs(t, err, context.Canceled)
}

// TestExecRunPostStart_RunsHookAndStreamsToListener verifies that
// execRunPostStart resolves the replica the start chain just brought up and
// runs its post_start hooks, forwarding their output to the listener the
// executor was constructed with — the plumbing an attached `up` will use
// once the interactive-up convergence wires a real one in.
func TestExecRunPostStart_RunsHookAndStreamsToListener(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	var mu sync.Mutex
	var lines []string
	listener := func(event api.ContainerEvent) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, event.Line)
	}

	serverConn, clientConn := net.Pipe()
	go func() {
		assert.NilError(t, writeStdcopyFrame(serverConn, 1, "post-start ok\n"))
		serverConn.Close() //nolint:errcheck
	}()

	apiClient.EXPECT().ExecCreate(gomock.Any(), "c1", gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec1"}, nil)
	apiClient.EXPECT().ExecAttach(gomock.Any(), "exec1", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil)
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec1", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 0}, nil)

	service := types.ServiceConfig{
		Name:      "web",
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"/notify.sh"}}},
	}
	ctr := container.Summary{ID: "c1", Names: []string{"/test-web-1"}}

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), listener)
	err := exec.execRunPostStart(t.Context(), Operation{Service: &service, Container: &ctr})
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, []string{"post-start ok"})
}

// TestExecutePlanStartPhaseEventContract drives a full wait + enriched-start
// plan through the real executor and asserts the per-resource progressions
// the event contract promises (see api.EventProcessor): a dependency wait
// reports Waiting→Healthy on the dependency's own container, and the
// dependent's start reports a single Starting→Started on its own container —
// pre_start/post_start, when declared, fold into that same progression
// rather than surfacing their own.
func TestExecutePlanStartPhaseEventContract(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"db":  {Name: "db"},
			"app": {Name: "app"},
		},
	}
	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	appSummary := container.Summary{
		ID:     "app-id",
		Names:  []string{"/test-app-1"},
		Labels: map[string]string{api.ServiceLabel: "app", api.OneoffLabel: "False"},
	}
	observed := emptyObservedState("test")

	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Healthy},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "app-id", gomock.Any()).Return(client.ContainerStartResult{}, nil)

	appSvc := project.Services["app"]
	plan := &Plan{}
	wait := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "app depends on db (service_healthy)",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:app:1",
		Cause:      "start",
		Service:    &appSvc,
		Container:  &appSummary,
	}, "start:app:1", wait)

	exec := svc.newPlanExecutor(project, observed, nil)
	exec.containersByService["db"] = Containers{dbSummary}

	assert.NilError(t, exec.run(t.Context(), plan))

	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting, api.StatusHealthy})
	assert.DeepEqual(t, recorder.byID["Container test-app-1"], []string{api.StatusStarting, api.StatusStarted})
}

// TestExecutePlanStartPhaseFullChainThroughCreateNode drives
// RunPreStart→StartContainer→RunPostStart together through the real run()
// DAG/group-tracker path for a replica the plan itself creates (no observed
// container.Summary — CreateNodeID only), exercising resolveContainerSummary's
// live-view lookup. It asserts the group emits exactly one Starting→Started
// pair, with Starting firing at the StartContainer node — not at
// RunPreStart, which must run silently first, matching the imperative
// engine's startService.
func TestExecutePlanStartPhaseFullChainThroughCreateNode(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name:      "web",
		PreStart:  []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"/notify.sh"}}},
	}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "new-id"}, nil)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:              "new-id",
			Name:            "/test-web-1",
			Config:          &container.Config{},
			NetworkSettings: &container.NetworkSettings{},
		}}, nil)
	// pre_start's daemon drift-check: no replica running yet, then the
	// runner scan finds the hook-0 runner the (untested here) create phase
	// prepared, and its wait→logs→start→remove sequence runs to success.
	driftCheck := apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)
	scan := expectRunnerScan(apiClient, runnerSummary("hook-1", 0)).After(driftCheck)
	wait := apiClient.EXPECT().ContainerWait(gomock.Any(), "hook-1", gomock.Any()).
		Return(waitResultExit(0)).After(scan)
	logs := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(wait)
	hookStart := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(logs)
	expectSuccessRemove(apiClient, "hook-1").After(hookStart)

	apiClient.EXPECT().ContainerStart(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ExecCreate(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec1"}, nil)
	apiClient.EXPECT().ExecAttach(gomock.Any(), "exec1", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ExecAttachOptions) (client.ExecAttachResult, error) {
			serverConn, clientConn := net.Pipe()
			go serverConn.Close() //nolint:errcheck
			return client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil
		})
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec1", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 0}, nil)

	plan := &Plan{}
	create := plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:web:1",
		Cause:      "no existing container",
		Service:    &service,
		Name:       "test-web-1",
		Number:     1,
	}, "")
	preStart := plan.addNode(Operation{
		Type:         OpRunPreStart,
		ResourceID:   "service:web:1",
		Cause:        "pre_start hooks",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", create)
	start := plan.addNode(Operation{
		Type:         OpStartContainer,
		ResourceID:   "service:web:1",
		Cause:        "start",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", preStart)
	plan.addNode(Operation{
		Type:         OpRunPostStart,
		ResourceID:   "service:web:1",
		Cause:        "post_start hooks",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", start)

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	assert.NilError(t, exec.run(t.Context(), plan))

	// The create node is ungrouped (Creating/Created), then the start group
	// contributes exactly one Starting/Started pair — pre_start and
	// post_start run silently within it.
	assert.DeepEqual(t, recorder.byID["Container test-web-1"], []string{"Creating", "Created", api.StatusStarting, api.StatusStarted})
}

// TestExecutePlanMissingRequiredDependencyFailsSilently verifies, through the
// real run()/emitErrorEvent path, that a required dependency with no live
// container at all fails the plan without emitting any spurious event —
// matching waitDependencies, which reports this case as a returned error
// only (see execWaitCondition's len(waitingFor)==0 branch).
func TestExecutePlanMissingRequiredDependencyFailsSilently(t *testing.T) {
	svc, _, recorder := newStartPhaseTestService(t)

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "app depends on db (service_healthy)",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")

	err := svc.executePlan(t.Context(), &types.Project{Name: "test"}, emptyObservedState("test"), plan)
	assert.ErrorContains(t, err, "missing dependency db")
	assert.Equal(t, len(recorder.byID), 0, "a missing-dependency failure must not emit a spurious event")
}

// TestExecutePlanFailedPreStartGatesStart is a regression test for a race in
// run()'s DAG walk: a node's done-channel closes (unblocking its dependents)
// inside the failing node's own goroutine, strictly before errgroup calls
// cancel() on that goroutine's returned error — cancel() only runs after it
// returns. A dependent unblocked from `<-done[dep.ID]` can therefore observe
// ctx.Err() == nil for a brief window even though its dependency just failed
// for a genuine reason, not a cancellation.
//
// execCreateHookContainer already narrows this with a leading ctx.Err()
// check (it cannot close the window — see the guard's comment in
// execStartContainer for why this is deferred to a dedicated executor-lot
// fix, #14081); this test pins the same narrowing on execStartContainer's
// enriched branch: when OpRunPreStart fails for a real reason (here: no hook
// runner container found — never a context cancellation), the OpStartContainer
// depending on it must not call ContainerStart. No ContainerStart expectation
// is registered below: an unexpected call fails the test.
func TestExecutePlanFailedPreStartGatesStart(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name:     "web",
		PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Command: types.ShellCommand{"init"}}}},
	}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "new-id"}, nil)
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "new-id", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:              "new-id",
			Name:            "/test-web-1",
			Config:          &container.Config{},
			NetworkSettings: &container.NetworkSettings{},
		}}, nil)
	// Both the pre_start drift-check and the hook-runner scan list no
	// container: no replica is running, and no runner was prepared for the
	// declared hook — runPreStart fails with "no hook runner container
	// found", a genuine error, not a cancellation.
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil).Times(2)
	// No ContainerStart expectation: registering one here would hide the
	// bug by silently accepting the call this test exists to forbid.

	plan := &Plan{}
	create := plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:web:1",
		Cause:      "no existing container",
		Service:    &service,
		Name:       "test-web-1",
		Number:     1,
	}, "")
	preStart := plan.addNode(Operation{
		Type:         OpRunPreStart,
		ResourceID:   "service:web:1",
		Cause:        "pre_start hooks",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", create)
	plan.addNode(Operation{
		Type:         OpStartContainer,
		ResourceID:   "service:web:1",
		Cause:        "start",
		Service:      &service,
		CreateNodeID: create.ID,
	}, "start:web:1", preStart)

	err := svc.executePlan(t.Context(), project, emptyObservedState("test"), plan)
	assert.ErrorContains(t, err, "no hook runner container found")

	// The group never got to fire its own Working/Starting event (triggered
	// only by OpStartContainer -- see triggersWorking), so onNodeError fires
	// it implicitly before Error, keeping the progression well-formed. And
	// precisely because OpStartContainer never ran (the assertion above),
	// there is no second, spurious Starting after it: this is the event-side
	// half of the race TestExecutePlanFailedPreStartGatesStart's name refers
	// to -- see run()'s failedDependency check.
	preStartErr := "service \"web\" pre_start[0]: no hook runner container found — " +
		"either its runner was already consumed by a previous start, or the " +
		"service's container state changed after reconciliation planned this " +
		"run; run `docker compose up web` to prepare fresh runners"
	assert.DeepEqual(t, recorder.byID["Container test-web-1"], []string{
		"Creating", "Created", api.StatusStarting, preStartErr,
	})
}
