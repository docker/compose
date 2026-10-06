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
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	err := exec.execWaitCondition(t.Context(), t.Context(), Operation{
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

	err := exec.execWaitCondition(t.Context(), t.Context(), Operation{
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

	err := exec.execWaitCondition(t.Context(), t.Context(), Operation{
		Name:      "db",
		Condition: types.ServiceConditionHealthy,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting, api.StatusHealthy})
}

// TestExecutePlanWaitConditionRespectsContextDeadline is a regression test
// for a gap a reviewer caught on upDetached (pkg/compose/up.go): the plan's
// own OpWaitCondition nodes have no timeout of their own (unlike the
// imperative engine's waitDependencies, which already threaded WaitTimeout
// through every per-dependency wait, not just the final --wait check) --
// they rely entirely on whatever ctx they're given (directly, or via
// exec.waitTimeout -- see TestExecutePlanWaitDeadlineOnlyBoundsWaitNodes for
// that path specifically). This drives a real OpWaitCondition node
// (service_healthy, never satisfied) through the full executePlan/run() DAG
// path -- not execWaitCondition in isolation -- under a short deadline on
// the ctx run() itself is given, and asserts it returns promptly with
// ctx.Err(), proving the deadline really does propagate from the caller into
// the plan's blocking nodes.
func TestExecutePlanWaitConditionRespectsContextDeadline(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Starting},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil).AnyTimes()

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := exec.run(ctx, plan)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Assert(t, elapsed < 5*time.Second, "execWaitCondition's polling loop did not stop at the deadline (took %s)", elapsed)
}

// TestExecutePlanWaitDeadlineOnlyBoundsWaitNodes is a regression test for a
// Copilot review finding on #14290: upDetached originally bounded
// --wait-timeout by wrapping the whole run() ctx, which would have also
// capped unrelated Create-phase and Start-phase work (network/container
// creation, hooks) that was never subject to --wait-timeout in the old
// create()+start() sequence. exec.waitTimeout (set by upDetached, consulted
// only in executeNode's OpWaitCondition case) must bound exclusively the
// blocking dependency wait, leaving every other node on the plan's plain,
// deadline-free ctx. This plan pairs a never-satisfied OpWaitCondition with
// an OpCreateNetwork that has no deadline of its own: if waitTimeout leaked
// into the whole run(), the network create would race the same short
// deadline instead of completing normally.
func TestExecutePlanWaitDeadlineOnlyBoundsWaitNodes(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Starting},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil).AnyTimes()
	nw := types.NetworkConfig{Name: "test_default"}
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{ID: "net-id"}, nil)

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Name:       nw.Name,
		Network:    &nw,
	}, "")
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	exec.waitTimeout = 200 * time.Millisecond

	start := time.Now()
	err := exec.run(t.Context(), plan)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Assert(t, elapsed < 5*time.Second, "waitTimeout did not bound the stuck OpWaitCondition node (took %s)", elapsed)
}

// TestExecutePlanOptionalWaitDoesNotSwallowInheritedDeadline is a regression
// test for a Copilot review finding on #14290: waitDependency originally
// tolerated any DeadlineExceeded on an optional dependency as long as the
// caller had configured a timeout at all (exec.waitTimeout > 0), regardless
// of whether THIS wait's own timeout, or an earlier deadline inherited from
// the caller's own ctx, was the one that actually fired. Both make
// ctx.Done() report DeadlineExceeded once the earlier of the two elapses, so
// a bare "did I configure a timeout" flag can't tell them apart -- an
// optional dependency would then silently swallow a real external deadline
// (e.g. the process's own context, or a test harness timeout) and report
// success. This plan's single OpWaitCondition node never resolves
// (BestEffort: true) and exec.waitTimeout is set far longer than the ctx
// passed into exec.run, so the INHERITED deadline is always the one that
// fires first: the error must still propagate, not be tolerated as if
// exec.waitTimeout itself had elapsed.
func TestExecutePlanOptionalWaitDoesNotSwallowInheritedDeadline(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Starting},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil).AnyTimes()

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
		BestEffort: true,
	}, "")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	exec.waitTimeout = 10 * time.Second

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := exec.run(ctx, plan)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, context.DeadlineExceeded, "an inherited deadline must not be swallowed just because this wait is optional and exec.waitTimeout is set")
	assert.Assert(t, elapsed < 5*time.Second, "took too long to fail on the inherited deadline (took %s)", elapsed)
}

// TestExecutePlanOptionalWaitTimeoutIsToleratedWithoutInheritedDeadline is
// the positive-case counterpart to
// TestExecutePlanOptionalWaitDoesNotSwallowInheritedDeadline: with no
// inherited deadline at all (plain t.Context() passed into exec.run), this
// wait's own exec.waitTimeout elapsing on an optional dependency must still
// be tolerated as before (nil, not an error) -- the origCtx refactor must
// not have turned every DeadlineExceeded into a hard failure.
func TestExecutePlanOptionalWaitTimeoutIsToleratedWithoutInheritedDeadline(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Starting},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		},
	}, nil).AnyTimes()

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
		BestEffort: true,
	}, "")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	exec.waitTimeout = 200 * time.Millisecond

	start := time.Now()
	err := exec.run(t.Context(), plan)
	elapsed := time.Since(start)

	assert.NilError(t, err, "exec.waitTimeout elapsing on its own, with no inherited deadline, must still be tolerated for an optional dependency")
	assert.Assert(t, elapsed < 5*time.Second, "took too long (took %s)", elapsed)
}

// TestExecutePlanCreatePhaseIsABarrierBeforeStartPhase is a regression test
// for a Copilot review finding on #14291: a Start-phase node's own DependsOn
// edges being satisfied is not enough to let it run -- nothing stopped it
// from starting while a COMPLETELY UNRELATED Create-phase node elsewhere in
// the plan was still in flight (or about to fail), something the old
// create()-then-start() sequence never allowed (the whole create phase,
// project-wide, always finished -- or failed, with start() never invoked at
// all -- before any dependency wait, or anything else in the start phase,
// even began). Interactive up's attach/printer session needs that same
// guarantee: it takes exclusive hold of the terminal for the create phase's
// progress display, handing it to continuous container log streaming only
// once the create phase is entirely done. This pairs a slow, unrelated
// network create (Phase: PhaseCreate, the default, no DependsOn edge to
// anything in the Start phase) with an OpStartContainer node (Phase:
// PhaseStart) and asserts ContainerStart is never called before the network
// create has fully returned. Uses OpStartContainer rather than
// OpWaitCondition deliberately: execWaitCondition's underlying poll loop
// only checks on a 500ms ticker (never on entry), which would make a
// same-order-of-magnitude Create delay pass this assertion even with no
// barrier at all, and so couldn't actually catch a regression here.
func TestExecutePlanCreatePhaseIsABarrierBeforeStartPhase(t *testing.T) {
	svc, apiClient := newTestService(t)

	var createDone atomic.Bool
	nw := types.NetworkConfig{Name: "test_default"}
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
			time.Sleep(100 * time.Millisecond)
			createDone.Store(true)
			return client.NetworkCreateResult{ID: "net-id"}, nil
		})

	apiClient.EXPECT().ContainerStart(gomock.Any(), "db-id", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			assert.Assert(t, createDone.Load(), "the Start-phase container start began before the unrelated Create-phase network create returned")
			return client.ContainerStartResult{}, nil
		})

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Name:       nw.Name,
		Network:    &nw,
	}, "")
	start := plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:db:1",
		Container:  &container.Summary{ID: "db-id", Names: []string{"/test-db-1"}},
	}, "")
	start.Phase = PhaseStart

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	assert.NilError(t, exec.run(t.Context(), plan))
	assert.Assert(t, createDone.Load())
}

// TestExecutePlanCreatePhaseFailureCancelsCreatePhase verifies phase-scoped
// cancellation didn't weaken the Create phase's own existing fail-fast
// behavior -- two independent (no DependsOn edge) Create-phase nodes, one
// failing immediately, must still cancel the other's in-flight work. Both
// nodes default to Phase: PhaseCreate (addNode's zero value), so they share
// the same runPhase call's phaseCancel.
func TestExecutePlanCreatePhaseFailureCancelsCreatePhase(t *testing.T) {
	svc, apiClient := newTestService(t)

	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, errors.New("boom"))

	var volumeCreateCtxErr error
	volumeCreateDone := make(chan struct{})
	apiClient.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ client.VolumeCreateOptions) (client.VolumeCreateResult, error) {
			defer close(volumeCreateDone)
			<-ctx.Done()
			volumeCreateCtxErr = ctx.Err()
			return client.VolumeCreateResult{}, ctx.Err()
		})

	nw := types.NetworkConfig{Name: "test_default"}
	vol := types.VolumeConfig{Name: "data", Driver: "local"}

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Name:       nw.Name,
		Network:    &nw,
	}, "")
	plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: "volume:data",
		Name:       vol.Name,
		Volume:     &vol,
	}, "")

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	err := exec.run(t.Context(), plan)
	assert.ErrorContains(t, err, "boom")

	select {
	case <-volumeCreateDone:
	case <-time.After(5 * time.Second):
		t.Fatal("VolumeCreate's ctx was never canceled -- a sibling Create-phase failure should still fail fast")
	}
	assert.ErrorIs(t, volumeCreateCtxErr, context.Canceled, "VolumeCreate's ctx must be canceled by a sibling Create-phase node failing")
}

// TestExecutePlanCreatePhaseFailureNeverDispatchesStartPhase complements
// TestExecutePlanCreatePhaseIsABarrierBeforeStartPhase: that test proves the
// Start phase waits for a successful Create phase, this one proves a failed
// Create phase skips the Start phase entirely rather than dispatching it and
// canceling it. run() must return the Create-phase error before runPhase is
// ever called on startNodes -- ContainerStart's .Times(0) fails the test the
// instant it's called at all, not just if it fails to be called.
func TestExecutePlanCreatePhaseFailureNeverDispatchesStartPhase(t *testing.T) {
	svc, apiClient := newTestService(t)

	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, errors.New("boom"))
	apiClient.EXPECT().ContainerStart(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	nw := types.NetworkConfig{Name: "test_default"}

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Name:       nw.Name,
		Network:    &nw,
	}, "")
	start := plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:db:1",
		Container:  &container.Summary{ID: "db-id", Names: []string{"/test-db-1"}},
	}, "")
	start.Phase = PhaseStart

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	err := exec.run(t.Context(), plan)
	assert.ErrorContains(t, err, "boom")
}

// TestExecutePlanConcurrentPhaseFailureKeepsOriginatingError exercises a
// Copilot/docker-agent finding on the phase-scoped cancellation fix:
// errgroup.Group's plain Wait() returns whichever goroutine's error reaches
// its internal sync.Once first, with no idea which one actually caused the
// others to fail -- a sibling unblocked by a phase's cancellation and
// returning a bare context.Canceled of its own could in principle win that
// race over the originating node's real error. phaseCancel's own sync.Once
// records the first failure explicitly, and run() prefers it over
// eg.Wait()'s own pick, closing that window regardless of goroutine
// scheduling. This test runs the scenario under real concurrency (one
// immediately-failing node alongside many siblings unblocked by its
// cancellation, via dependency-wait on a PlanNode ID never added to the
// plan -- rs.done[dep.ID] is then a nil map entry, so the only way out of
// the select is <-nodeCtx.Done(), firing the instant the phase cancels) and
// asserts the real error always surfaces. Note: in practice the Go runtime
// never actually lets a sibling win this race in this in-process test --
// the originating goroutine keeps running uninterrupted through its own
// short return path before the scheduler gets around to any of the
// newly-runnable siblings -- so reverting the fix does not make this test
// fail; its value is exercising the mechanism under load, not proving the
// fix by ablation. The fix's correctness instead rests on reading
// errgroup's own errOnce.Do source directly (golang.org/x/sync/errgroup),
// confirmed independently by two reviewers.
func TestExecutePlanConcurrentPhaseFailureKeepsOriginatingError(t *testing.T) {
	svc, _ := newTestService(t)

	plan := &Plan{}
	fail := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:missing:service_healthy",
		Name:       "missing", // no registered containers under this name -> immediate failure
		Condition:  types.ServiceConditionHealthy,
	}, "")
	fail.Phase = PhaseStart

	phantom := &PlanNode{ID: -1} // never added to plan.Nodes; its done-channel is never created
	const siblings = 30
	for i := range siblings {
		blocked := plan.addNode(Operation{
			Type:       OpWaitCondition,
			ResourceID: fmt.Sprintf("wait:phantom:%d", i),
			Name:       "phantom",
		}, "", phantom)
		blocked.Phase = PhaseStart
	}

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)

	err := exec.run(t.Context(), plan)
	assert.ErrorContains(t, err, "missing dependency missing")
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
