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
	"runtime"
	"slices"
	"strings"
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
	// the network create must run on a context with no deadline at all: the
	// wait's own window is derived for the OpWaitCondition node only, so a
	// deadline showing up here would mean waitTimeout leaked into the plan
	var networkCtxHadDeadline atomic.Bool
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
			_, has := ctx.Deadline()
			networkCtxHadDeadline.Store(has)
			return client.NetworkCreateResult{ID: "net-id"}, nil
		})

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
	assert.Assert(t, !networkCtxHadDeadline.Load(), "waitTimeout leaked into a non-wait node's context")
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

	// run waits for every node, and VolumeCreate blocks until its ctx ends: if
	// the sibling failure ever stopped canceling it, run would hang until the
	// suite-wide timeout. This deadline turns that into a local failure, and
	// the context.Canceled assertion below tells it apart from the sibling
	// cancellation the test is about.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := exec.run(ctx, plan)
	assert.ErrorContains(t, err, "boom")

	select {
	case <-volumeCreateDone:
	default:
		t.Fatal("run returned before VolumeCreate did")
	}
	assert.ErrorIs(t, volumeCreateCtxErr, context.Canceled, "VolumeCreate's ctx must be canceled by a sibling Create-phase node failing, not by the test's own deadline")
}

// TestExecutePlanCanceledDependencyIsRecordedAsFailure pins the failure a
// node records when it leaves its dependency wait because its phase was
// canceled. In X -> A -> B with X failing without any I/O, A bails out on the
// cancellation and its done channel is closed; B may select that channel
// before it sees the cancellation itself. With no failure recorded for A, B
// then went on to start and enter executeNode on a canceled context. B must
// be skipped. The bad interleaving needs B to reach its select late, when
// both channels are already ready and the select picks one at random: a
// single P makes that the usual case, and the repetition covers the rest of
// the randomness.
func TestExecutePlanCanceledDependencyIsRecordedAsFailure(t *testing.T) {
	prev := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	svc, apiClient := newTestService(t)

	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		Return(client.NetworkCreateResult{}, errors.New("boom")).AnyTimes()
	apiClient.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Times(0)

	nw := types.NetworkConfig{Name: "test_default"}
	volA := types.VolumeConfig{Name: "a", Driver: "local"}
	volB := types.VolumeConfig{Name: "b", Driver: "local"}

	for range 200 {
		plan := &Plan{}
		x := plan.addNode(Operation{Type: OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")
		a := plan.addNode(Operation{Type: OpCreateVolume, ResourceID: "volume:a", Name: volA.Name, Volume: &volA}, "", x)
		plan.addNode(Operation{Type: OpCreateVolume, ResourceID: "volume:b", Name: volB.Name, Volume: &volB}, "", a)

		exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
		assert.ErrorContains(t, exec.run(t.Context(), plan), "boom")
	}
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
	}, "start:web:1", rename).Phase = PhaseStart

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

// TestExecutePlanStartInjectsSecretsAndConfigsBeforeContainerStart pins that
// a start node copies the service's secrets and configs into the container
// before starting it, in that order.
func TestExecutePlanStartInjectsSecretsAndConfigsBeforeContainerStart(t *testing.T) {
	svc, apiClient, _ := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name: "web",
		ContainerSpec: types.ContainerSpec{
			Secrets: []types.ServiceSecretConfig{{Source: "token"}},
			Configs: []types.ServiceConfigObjConfig{{Source: "settings"}},
		},
	}
	project := &types.Project{
		Name:     "test",
		Services: types.Services{"web": service},
		Secrets:  types.Secrets{"token": types.SecretConfig{Name: "token", Content: "s3cret"}},
		Configs:  types.Configs{"settings": types.ConfigObjConfig{Name: "settings", Content: "key=value"}},
	}
	ctr := container.Summary{ID: "web-id", Names: []string{"/test-web-1"}}

	secret := apiClient.EXPECT().CopyToContainer(gomock.Any(), "web-id", gomock.Any()).
		Return(client.CopyToContainerResult{}, nil)
	config := apiClient.EXPECT().CopyToContainer(gomock.Any(), "web-id", gomock.Any()).
		Return(client.CopyToContainerResult{}, nil).After(secret)
	apiClient.EXPECT().ContainerStart(gomock.Any(), "web-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(config)

	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:web:1",
		Cause:      "start",
		Service:    &service,
		Container:  &ctr,
	}, "start:web:1")

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	assert.NilError(t, exec.run(t.Context(), plan))
}

// TestExecutePlanFailedPostStartFailsTheStart pins that a failing post_start
// hook fails the plan and leaves the container's progression without its
// Started event.
func TestExecutePlanFailedPostStartFailsTheStart(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	service := types.ServiceConfig{
		Name:      "web",
		PostStart: []types.ServiceHook{{Command: types.ShellCommand{"fail"}}},
	}
	ctr := container.Summary{ID: "web-id", Names: []string{"/test-web-1"}}

	apiClient.EXPECT().ContainerStart(gomock.Any(), "web-id", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)
	apiClient.EXPECT().ExecCreate(gomock.Any(), "web-id", gomock.Any()).
		Return(client.ExecCreateResult{ID: "exec-1"}, nil)
	serverConn, clientConn := net.Pipe()
	_ = serverConn.Close()
	apiClient.EXPECT().ExecAttach(gomock.Any(), "exec-1", gomock.Any()).
		Return(client.ExecAttachResult{HijackedResponse: client.NewHijackedResponse(clientConn, "")}, nil)
	apiClient.EXPECT().ExecInspect(gomock.Any(), "exec-1", gomock.Any()).
		Return(client.ExecInspectResult{ExitCode: 3}, nil)

	plan := &Plan{}
	start := plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:web:1",
		Cause:      "start",
		Service:    &service,
		Container:  &ctr,
	}, "start:web:1")
	plan.addNode(Operation{
		Type:       OpRunPostStart,
		ResourceID: "service:web:1",
		Cause:      "post_start hooks",
		Service:    &service,
		Container:  &ctr,
	}, "start:web:1", start)

	exec := svc.newPlanExecutor(&types.Project{Name: "test", Services: types.Services{"web": service}}, emptyObservedState("test"), nil)
	err := exec.run(t.Context(), plan)
	assert.ErrorContains(t, err, "hook exited with status 3")
	assert.Assert(t, !slices.Contains(recorder.byID["Container test-web-1"], api.StatusStarted),
		"a failed post_start must not report Started: %v", recorder.byID["Container test-web-1"])
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
	hookStart := apiClient.EXPECT().ContainerStart(gomock.Any(), "hook-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil).After(wait)
	logs := apiClient.EXPECT().ContainerLogs(gomock.Any(), "hook-1", gomock.Any()).
		Return(emptyLogs(), nil).After(hookStart)
	expectSuccessRemove(apiClient, "hook-1").After(logs)

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

// TestExecutePlanWaitTimeoutIsMarked verifies that a wait node's own
// --wait-timeout window expiring is reported as errWaitTimeout -- the marker
// upDetached and start() translate into a user-facing message -- while still
// satisfying context.DeadlineExceeded.
func TestExecutePlanWaitTimeoutIsMarked(t *testing.T) {
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
	exec.waitTimeout = 200 * time.Millisecond

	err := exec.run(t.Context(), plan)
	assert.ErrorIs(t, err, errWaitTimeout)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestExecutePlanOtherDeadlinesAreNotWaitTimeouts verifies that a
// DeadlineExceeded that is not a wait node's own window expiring is never
// marked errWaitTimeout, even with --wait-timeout set: an engine call timing
// out in a node that is not a wait, and a deadline inherited from the
// caller's context firing during a wait. Translating either into a
// dependency-readiness message would blame the wrong thing.
func TestExecutePlanOtherDeadlinesAreNotWaitTimeouts(t *testing.T) {
	t.Run("engine call timing out in a non-wait node", func(t *testing.T) {
		svc, apiClient, _ := newStartPhaseTestService(t)
		apiClient.EXPECT().ContainerStart(gomock.Any(), "db-id", gomock.Any()).
			Return(client.ContainerStartResult{}, context.DeadlineExceeded)

		plan := &Plan{}
		plan.addNode(Operation{
			Type:       OpStartContainer,
			ResourceID: "service:db:1",
			Container:  &container.Summary{ID: "db-id", Names: []string{"/test-db-1"}},
		}, "").Phase = PhaseStart

		exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
		exec.waitTimeout = time.Minute

		err := exec.run(t.Context(), plan)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Assert(t, !errors.Is(err, errWaitTimeout), "an engine-call timeout was marked as a wait timeout: %v", err)
	})

	t.Run("deadline inherited from the caller during a wait", func(t *testing.T) {
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
		exec.waitTimeout = time.Minute // far longer than the caller's own deadline

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		err := exec.run(ctx, plan)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Assert(t, !errors.Is(err, errWaitTimeout), "an inherited deadline was marked as a wait timeout: %v", err)
	})
}

// TestExecutePlanWaitTimeoutSurvivesDependentSkip verifies the marker is still
// what run() returns when a node depends on the wait that timed out: the
// dependent is skipped with "dependency failed: ...", but it is the wait's own
// failure that is reported, so the caller still translates it.
func TestExecutePlanWaitTimeoutSurvivesDependentSkip(t *testing.T) {
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
	// no ContainerStart expectation: the dependent must never run

	plan := &Plan{}
	wait := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	// both in the Start phase, like the reconciler plans them: the skipped
	// dependent only exists if the wait that fails is in its own phase
	wait.Phase = PhaseStart
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:app:1",
		Container:  &container.Summary{ID: "app-id", Names: []string{"/test-app-1"}},
	}, "", wait).Phase = PhaseStart

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	exec.waitTimeout = 200 * time.Millisecond

	err := exec.run(t.Context(), plan)
	assert.ErrorIs(t, err, errWaitTimeout)
}

func TestTranslateWaitTimeout(t *testing.T) {
	marked := fmt.Errorf("%w: %w", errWaitTimeout, context.DeadlineExceeded)
	other := errors.New("boom")

	// a depends_on wait timing out is a dependency timeout, whether or not
	// --wait was given: "application not healthy" is the final readiness
	// check's message, not this one's
	assert.Error(t, translateWaitTimeout(marked), "timeout waiting for dependencies")

	// anything that is not a wait's own timeout comes back untouched: a bare
	// DeadlineExceeded included
	assert.Equal(t, translateWaitTimeout(other), other)
	assert.Equal(t, translateWaitTimeout(context.DeadlineExceeded), context.DeadlineExceeded)
	assert.NilError(t, translateWaitTimeout(nil))
}

// startFailureOptions varies startFailureFixture.
type startFailureOptions struct {
	// phase is the phase every node of the plan belongs to
	phase PlanPhase
	// inspectBlocks makes the inspection of db wait for the end of its context
	// and fail with it, as one cut short by the cancellation does
	inspectBlocks bool
}

// startFailureFixture builds a project where web fails to start while api,
// which depends on db being healthy, is still held by that wait. db's health
// never changes, so only web's failure can end the phase.
func startFailureFixture(t *testing.T, opts startFailureOptions) (*recordingEvents, *planExecutor, *Plan) {
	t.Helper()
	svc, apiClient, recorder := newStartPhaseTestService(t)

	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"db":  {Name: "db"},
			"api": {Name: "api"},
			"web": {Name: "web"},
		},
	}
	summary := func(service string) container.Summary {
		return container.Summary{
			ID:     service + "-id",
			Names:  []string{"/test-" + service + "-1"},
			Labels: map[string]string{api.ServiceLabel: service, api.OneoffLabel: "False"},
		}
	}
	dbSummary, apiSummary, webSummary := summary("db"), summary("api"), summary("web")

	// web fails once the wait on db is polling, so that the failure finds the
	// wait in flight rather than not dispatched yet
	polling := make(chan struct{})
	var pollingOnce sync.Once
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			pollingOnce.Do(func() { close(polling) })
			if opts.inspectBlocks {
				<-ctx.Done()
				return client.ContainerInspectResult{}, fmt.Errorf("inspect db-id: %w", ctx.Err())
			}
			return client.ContainerInspectResult{Container: container.InspectResponse{
				ID:   "db-id",
				Name: "/test-db-1",
				State: &container.State{
					Status: container.StateRunning,
					Health: &container.Health{Status: container.Starting},
				},
				Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
			}}, nil
		}).AnyTimes()
	apiClient.EXPECT().ContainerStart(gomock.Any(), "web-id", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			select {
			case <-polling:
			case <-time.After(5 * time.Second):
				t.Error("the wait on db never started polling")
			}
			return client.ContainerStartResult{}, errors.New("port is already allocated")
		})
	// no ContainerStart for api: it must never be reached

	apiSvc, webSvc := project.Services["api"], project.Services["web"]
	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:web:1",
		Cause:      "start",
		Service:    &webSvc,
		Container:  &webSummary,
	}, "start:web:1")
	wait := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Cause:      "api depends on db (service_healthy)",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:api:1",
		Cause:      "start",
		Service:    &apiSvc,
		Container:  &apiSummary,
	}, "start:api:1", wait)
	for _, n := range plan.Nodes {
		n.Phase = opts.phase
	}

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	return recorder, exec, plan
}

// TestExecutePlanStartFailureReportsTheServicesItInterrupts pins what the rows
// of the other services say when a start fails: a service held by a depends_on
// wait is reported as not started, with the condition that was not reached,
// and the row of the dependency it was waiting on no longer reads "Waiting".
// A service recreated by the same command would otherwise be left on
// "Recreated" while it is never started.
func TestExecutePlanStartFailureReportsTheServicesItInterrupts(t *testing.T) {
	recorder, exec, plan := startFailureFixture(t, startFailureOptions{phase: PhaseStart})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := exec.run(ctx, plan)
	assert.ErrorContains(t, err, "port is already allocated")

	const reason = `condition "service_healthy" of service "db" not reached: interrupted by the failure of Container test-web-1`
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	web := recorder.byID["Container test-web-1"]
	assert.Equal(t, len(web), 2, "web: %v", web)
	assert.Equal(t, web[0], api.StatusStarting)
	assert.ErrorContains(t, errors.New(web[1]), "port is already allocated")
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting, "Skipped: " + reason})
	assert.DeepEqual(t, recorder.byID["Container test-api-1"], []string{"not started: " + reason})
}

// TestGroupTrackerOnNodeAborted pins the final event of a group given up
// because another node failed: "not started" while the group never reported
// Working, the bare interruption once it did, and never a second final event,
// before or after.
func TestGroupTrackerOnNodeAborted(t *testing.T) {
	summary := container.Summary{ID: "api-id", Names: []string{"/test-api-1"}}
	newTracker := func() (*groupTracker, *PlanNode, *capturingEvents) {
		plan := &Plan{}
		node := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Container: &summary}, "start:api:1")
		return (&planExecutor{}).buildGroupTracker(plan), node, &capturingEvents{}
	}
	const row = "Container test-api-1"

	t.Run("never started", func(t *testing.T) {
		gt, node, rec := newTracker()
		gt.onNodeAborted(node, rec, "interrupted")
		gt.onNodeAborted(node, rec, "interrupted")
		assert.DeepEqual(t, rec.resources, []api.Resource{{ID: row, Status: api.Error, Text: "not started: interrupted"}})
	})
	t.Run("started", func(t *testing.T) {
		gt, node, rec := newTracker()
		gt.onNodeStart(node, rec)
		gt.onNodeAborted(node, rec, "interrupted")
		gt.onNodeDone(node, rec)
		assert.DeepEqual(t, rec.resources, []api.Resource{
			{ID: row, Status: api.Working, Text: api.StatusStarting},
			{ID: row, Status: api.Error, Text: "interrupted"},
		})
	})
	t.Run("already failed", func(t *testing.T) {
		gt, node, rec := newTracker()
		gt.onNodeError(node, rec, errors.New("boom"))
		gt.onNodeAborted(node, rec, "interrupted")
		assert.DeepEqual(t, rec.resources, []api.Resource{
			{ID: row, Status: api.Working, Text: api.StatusStarting},
			{ID: row, Status: api.Error, Text: "boom"},
		})
	})
}

// TestExecutePlanCanceledCallerReportsNothingMore keeps what a canceled caller
// (Ctrl-C) gets as it was: the plan ends, and no row is rewritten as the
// victim of a failure there was none of.
func TestExecutePlanCanceledCallerReportsNothingMore(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	project := &types.Project{Name: "test", Services: types.Services{"db": {Name: "db"}, "api": {Name: "api"}}}
	dbSummary := container.Summary{
		ID:     "db-id",
		Names:  []string{"/test-db-1"},
		Labels: map[string]string{api.ServiceLabel: "db", api.OneoffLabel: "False"},
	}
	apiSummary := container.Summary{
		ID:     "api-id",
		Names:  []string{"/test-api-1"},
		Labels: map[string]string{api.ServiceLabel: "api", api.OneoffLabel: "False"},
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			cancel()
			return client.ContainerInspectResult{Container: container.InspectResponse{
				ID:   "db-id",
				Name: "/test-db-1",
				State: &container.State{
					Status: container.StateRunning,
					Health: &container.Health{Status: container.Starting},
				},
				Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
			}}, nil
		}).AnyTimes()

	apiSvc := project.Services["api"]
	plan := &Plan{}
	wait := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:api:1",
		Service:    &apiSvc,
		Container:  &apiSummary,
	}, "start:api:1", wait)
	for _, n := range plan.Nodes {
		n.Phase = PhaseStart
	}

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	_ = exec.run(ctx, plan)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting})
	assert.Equal(t, len(recorder.byID["Container test-api-1"]), 0, "api: %v", recorder.byID["Container test-api-1"])
}

// TestExecutePlanStartFailureClosesAnInterruptedInspection is the same start
// failure, found by a wait whose inspection of the dependency is in flight: it
// ends with the cancellation, which is the interruption, not a failure of the
// healthy dependency.
func TestExecutePlanStartFailureClosesAnInterruptedInspection(t *testing.T) {
	recorder, exec, plan := startFailureFixture(t, startFailureOptions{phase: PhaseStart, inspectBlocks: true})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := exec.run(ctx, plan)
	assert.ErrorContains(t, err, "port is already allocated")

	const reason = `condition "service_healthy" of service "db" not reached: interrupted by the failure of Container test-web-1`
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting, "Skipped: " + reason})
	assert.DeepEqual(t, recorder.byID["Container test-api-1"], []string{"not started: " + reason})
}

// TestExecutePlanCreatePhaseFailureReportsNothingOnItsSiblings keeps the create
// phase as it was: the nodes it gives up are not reported, only the failing
// one is.
func TestExecutePlanCreatePhaseFailureReportsNothingOnItsSiblings(t *testing.T) {
	recorder, exec, plan := startFailureFixture(t, startFailureOptions{phase: PhaseCreate})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := exec.run(ctx, plan)
	assert.ErrorContains(t, err, "port is already allocated")

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.Equal(t, len(recorder.byID["Container test-api-1"]), 0, "api: %v", recorder.byID["Container test-api-1"])
}

// TestExecutePlanFailedWaitNamesTheConditionNotMet pins what the service held
// by a wait reads when that wait is what fails: the dependency did not reach
// the condition, nothing was interrupted, and the wait node is not a resource
// of the output.
func TestExecutePlanFailedWaitNamesTheConditionNotMet(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	project := &types.Project{Name: "test", Services: types.Services{"db": {Name: "db"}, "api": {Name: "api"}}}
	dbSummary := startTestSummary("db")
	apiSummary := startTestSummary("api")
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).
		Return(client.ContainerInspectResult{Container: container.InspectResponse{
			ID:   "db-id",
			Name: "/test-db-1",
			State: &container.State{
				Status: container.StateRunning,
				Health: &container.Health{Status: container.Unhealthy},
			},
			Config: &container.Config{Healthcheck: &container.HealthConfig{Test: []string{"CMD", "true"}}},
		}}, nil).AnyTimes()

	apiSvc := project.Services["api"]
	plan := &Plan{}
	wait := plan.addNode(Operation{
		Type:       OpWaitCondition,
		ResourceID: "wait:db:service_healthy",
		Name:       "db",
		Condition:  types.ServiceConditionHealthy,
	}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:api:1",
		Service:    &apiSvc,
		Container:  &apiSummary,
	}, "start:api:1", wait)
	for _, n := range plan.Nodes {
		n.Phase = PhaseStart
	}

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{dbSummary}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := exec.run(ctx, plan)
	assert.ErrorContains(t, err, "dependency failed to start")

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.DeepEqual(t, recorder.byID["Container test-api-1"],
		[]string{`not started: condition "service_healthy" of service "db" not met`})
	for id, texts := range recorder.byID {
		assert.Assert(t, !strings.HasPrefix(id, "wait:"), "event on %q: %v", id, texts)
		for _, text := range texts {
			assert.Assert(t, !strings.Contains(text, "wait:"), "%s: %q", id, text)
		}
	}
}

// startTestSummary is the container of a service's first replica.
func startTestSummary(service string) container.Summary {
	return container.Summary{
		ID:     service + "-id",
		Names:  []string{"/test-" + service + "-1"},
		Labels: map[string]string{api.ServiceLabel: service, api.OneoffLabel: "False"},
	}
}

// concurrentFailure runs the creation of two networks, which do not depend on
// each other, to the point where both are in flight; fail then tells how each
// one ends. (Container starts cannot stand for them: they are serialized, see
// startMx.)
func concurrentFailure(t *testing.T, fail func(ctx context.Context, network string) error) (*recordingEvents, error) {
	t.Helper()
	svc, apiClient, recorder := newStartPhaseTestService(t)

	inFlight := map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{})}
	plan := &Plan{}
	for _, name := range []string{"a", "b"} {
		nw := types.NetworkConfig{Name: "test_" + name}
		apiClient.EXPECT().NetworkCreate(gomock.Any(), nw.Name, gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
				close(inFlight[name])
				// the other creation is in flight before either ends
				other := map[string]string{"a": "b", "b": "a"}[name]
				select {
				case <-inFlight[other]:
				case <-time.After(5 * time.Second):
					t.Error("the other creation never began")
				}
				return client.NetworkCreateResult{}, fail(ctx, name)
			})
		plan.addNode(Operation{
			Type:       OpCreateNetwork,
			ResourceID: "network:" + name,
			Name:       nw.Name,
			Network:    &nw,
		}, "")
	}

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	return recorder, exec.run(ctx, plan)
}

// TestExecutePlanConcurrentFailuresEachEndTheirRow pins that two nodes failing
// together both say why: the failure that cancels the phase is the one
// returned, but the other row must not be left on its Working status.
func TestExecutePlanConcurrentFailuresEachEndTheirRow(t *testing.T) {
	recorder, err := concurrentFailure(t, func(_ context.Context, network string) error {
		return errors.New("failed " + network)
	})
	assert.Assert(t, err != nil)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, network := range []string{"a", "b"} {
		assert.DeepEqual(t, recorder.byID["network:"+network], []string{"failed to create network test_" + network + ": failed " + network})
	}
}

// TestExecutePlanFailingOnItsOwnWhileCanceledSaysSo pins that a node which
// fails for a reason of its own after another one canceled the phase reports
// that reason, whichever of the two failures comes first.
func TestExecutePlanFailingOnItsOwnWhileCanceledSaysSo(t *testing.T) {
	recorder, err := concurrentFailure(t, func(ctx context.Context, network string) error {
		if network == "a" {
			return errors.New("failed a")
		}
		<-ctx.Done()
		return errors.New("failed b")
	})
	assert.ErrorContains(t, err, "failed a")

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.DeepEqual(t, recorder.byID["network:a"], []string{"failed to create network test_a: failed a"})
	assert.DeepEqual(t, recorder.byID["network:b"], []string{"failed to create network test_b: failed b"})
}

// TestAbortReasonOfPostStart pins that a replica whose start is behind it is
// not reported as one that never started: only its post_start hooks were cut
// short.
func TestAbortReasonOfPostStart(t *testing.T) {
	web, apiCtr := startTestSummary("web"), startTestSummary("api")
	plan := &Plan{}
	failed := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:web:1", Container: &web}, "start:web:1")
	start := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Container: &apiCtr}, "start:api:1")
	postStart := plan.addNode(Operation{Type: OpRunPostStart, ResourceID: "post_start:api:1", Container: &apiCtr}, "start:api:1", start)

	exec := &planExecutor{}
	rs := &runState{groups: exec.buildGroupTracker(plan), succeeded: func(int) bool { return true }}
	assert.Equal(t, exec.abortReason(start, rs, failed), "interrupted by the failure of Container test-web-1")
	assert.Equal(t, exec.abortReason(postStart, rs, failed), "started, post_start hooks interrupted by the failure of Container test-web-1")
}

// TestAbortReasonOfAFailedWait pins what a node reads when the node that ended
// the phase is a wait: held by that wait, it was not interrupted, its
// dependency did not meet the condition; any other node was interrupted by it.
func TestAbortReasonOfAFailedWait(t *testing.T) {
	apiCtr, webCtr := startTestSummary("api"), startTestSummary("web")
	plan := &Plan{}
	wait := plan.addNode(Operation{Type: OpWaitCondition, ResourceID: "wait:db:service_healthy", Name: "db", Condition: types.ServiceConditionHealthy}, "")
	held := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Container: &apiCtr}, "start:api:1", wait)
	other := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:web:1", Container: &webCtr}, "start:web:1")
	postStart := plan.addNode(Operation{Type: OpRunPostStart, ResourceID: "post_start:web:1", Container: &webCtr}, "start:web:1", other)

	exec := &planExecutor{}
	rs := &runState{groups: exec.buildGroupTracker(plan), succeeded: func(int) bool { return false }}
	assert.Equal(t, exec.abortReason(held, rs, wait), `condition "service_healthy" of service "db" not met`)
	assert.Equal(t, exec.abortReason(other, rs, wait), `interrupted by the unmet condition "service_healthy" of service "db"`)
	// its start did not succeed: nothing is started for the hooks to be cut short
	assert.Equal(t, exec.abortReason(postStart, rs, wait), `interrupted by the unmet condition "service_healthy" of service "db"`)
}

// TestExecutePlanCanceledCallerMidInspectionKeepsItsError keeps what a caller
// canceled (Ctrl-C) while a required wait inspects its dependency as it was: the
// wait fails, so the nodes it holds are skipped, and the healthy dependency is
// not reported as failed.
func TestExecutePlanCanceledCallerMidInspectionKeepsItsError(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	project := &types.Project{Name: "test", Services: types.Services{"db": {Name: "db"}, "api": {Name: "api"}}}
	apiCtr := startTestSummary("api")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-id", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
			cancel()
			return client.ContainerInspectResult{}, fmt.Errorf("inspect db-id: %w", context.Canceled)
		}).AnyTimes()

	apiSvc := project.Services["api"]
	plan := &Plan{}
	wait := plan.addNode(Operation{Type: OpWaitCondition, ResourceID: "wait:db:service_healthy", Name: "db", Condition: types.ServiceConditionHealthy}, "")
	plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Service: &apiSvc, Container: &apiCtr}, "start:api:1", wait)
	for _, n := range plan.Nodes {
		n.Phase = PhaseStart
	}

	exec := svc.newPlanExecutor(project, emptyObservedState("test"), nil)
	exec.containersByService["db"] = Containers{startTestSummary("db")}
	err := exec.run(ctx, plan)
	assert.ErrorIs(t, err, context.Canceled)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.DeepEqual(t, recorder.byID["Container test-db-1"], []string{api.StatusWaiting})
	assert.Equal(t, len(recorder.byID["Container test-api-1"]), 0, "api: %v", recorder.byID["Container test-api-1"])
}

// TestAbortReasonFollowsTheChainOfPrerequisites pins that the reason a node
// reads does not depend on which node of a chain reports first: wait -> start
// -> post_start is held by the wait for each of the three, and a post_start is
// only "started" once its start has succeeded.
func TestAbortReasonFollowsTheChainOfPrerequisites(t *testing.T) {
	apiCtr, webCtr := startTestSummary("api"), startTestSummary("web")
	plan := &Plan{}
	wait := plan.addNode(Operation{Type: OpWaitCondition, ResourceID: "wait:db:service_healthy", Name: "db", Condition: types.ServiceConditionHealthy}, "")
	start := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Container: &apiCtr}, "start:api:1", wait)
	postStart := plan.addNode(Operation{Type: OpRunPostStart, ResourceID: "post_start:api:1", Container: &apiCtr}, "start:api:1", start)
	failed := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:web:1", Container: &webCtr}, "start:web:1")

	exec := &planExecutor{}
	succeeded := map[int]bool{}
	rs := &runState{groups: exec.buildGroupTracker(plan), succeeded: func(id int) bool { return succeeded[id] }}
	const reason = `condition "service_healthy" of service "db" not reached: interrupted by the failure of Container test-web-1`
	assert.Equal(t, exec.abortReason(start, rs, failed), reason)
	assert.Equal(t, exec.abortReason(postStart, rs, failed), reason)

	// the wait is over and the start succeeded: only the hooks were cut short
	succeeded[wait.ID], succeeded[start.ID] = true, true
	assert.Equal(t, exec.abortReason(postStart, rs, failed), "started, post_start hooks interrupted by the failure of Container test-web-1")
}

// TestGroupTrackerNothingStartsAfterTheFinalEvent pins the order of a group's
// events when another node of the group gives it up while a node is still on
// its way to start: the final event stays the last one.
func TestGroupTrackerNothingStartsAfterTheFinalEvent(t *testing.T) {
	summary := container.Summary{ID: "api-id", Names: []string{"/test-api-1"}}
	plan := &Plan{}
	start := plan.addNode(Operation{Type: OpStartContainer, ResourceID: "service:api:1", Container: &summary}, "start:api:1")
	postStart := plan.addNode(Operation{Type: OpRunPostStart, ResourceID: "post_start:api:1", Container: &summary}, "start:api:1", start)
	gt := (&planExecutor{}).buildGroupTracker(plan)
	rec := &capturingEvents{}

	gt.onNodeAborted(postStart, rec, "interrupted")
	assert.Assert(t, !gt.onNodeStart(start, rec), "a node of a given up group must not go on")
	assert.DeepEqual(t, rec.resources, []api.Resource{{ID: "Container test-api-1", Status: api.Error, Text: "not started: interrupted"}})
}

// TestPlanRunDrivesPhasesSeparately verifies that begin/runCreate/runStart
// leave a real gap between the two phases: after runCreate returned, the
// Create-phase node has fully run and no Start-phase node was dispatched --
// the window interactive up uses to set up its attach/printer session. The
// Start-phase node depends on the Create-phase one, so the state the two
// phases share is exercised across the gap.
func TestPlanRunDrivesPhasesSeparately(t *testing.T) {
	svc, apiClient := newTestService(t)

	var networkCreated, started atomic.Bool
	nw := types.NetworkConfig{Name: "test_default"}
	apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
			networkCreated.Store(true)
			return client.NetworkCreateResult{ID: "net-id"}, nil
		})
	apiClient.EXPECT().ContainerStart(gomock.Any(), "db-id", gomock.Any()).
		DoAndReturn(func(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error) {
			started.Store(true)
			return client.ContainerStartResult{}, nil
		})

	plan := &Plan{}
	network := plan.addNode(Operation{Type: OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")
	plan.addNode(Operation{
		Type:       OpStartContainer,
		ResourceID: "service:db:1",
		Container:  &container.Summary{ID: "db-id", Names: []string{"/test-db-1"}},
	}, "", network).Phase = PhaseStart

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	pr, err := exec.begin(plan)
	assert.NilError(t, err)
	assert.Assert(t, !networkCreated.Load() && !started.Load(), "begin must not dispatch anything")

	assert.NilError(t, pr.runCreate(t.Context()))
	assert.Assert(t, networkCreated.Load(), "the Create phase must be complete once runCreate returned")
	assert.Assert(t, !started.Load(), "no Start-phase node may be dispatched before runStart")

	assert.NilError(t, pr.runStart(t.Context(), nil))
	assert.Assert(t, started.Load())
}

// hookLogs collects the lines of the hook logs a listener receives.
type hookLogs struct {
	mu    sync.Mutex
	lines []string
}

func (h *hookLogs) listener(event api.ContainerEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, event.Line)
}

func (h *hookLogs) got() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.lines)
}

// postStartHookPlan mocks the exec round trip of a post_start hook printing
// "post-start ok", and returns the service running it and the plan of its
// Start phase.
func postStartHookPlan(t *testing.T) (*composeService, *Plan) {
	svc, apiClient, _ := newStartPhaseTestService(t)

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
	plan := &Plan{}
	plan.addNode(Operation{
		Type:       OpRunPostStart,
		ResourceID: "service:web:1",
		Service:    &service,
		Container:  &container.Summary{ID: "c1", Names: []string{"/test-web-1"}},
	}, "").Phase = PhaseStart
	return svc, plan
}

// TestPlanRunStartTakesTheListenerAtPhaseEntry verifies that a listener
// handed to runStart -- not the one the executor was built with, nil here --
// receives the hook logs of the Start phase: an attached caller only has its
// printer once its session is set up, after the Create phase.
func TestPlanRunStartTakesTheListenerAtPhaseEntry(t *testing.T) {
	svc, plan := postStartHookPlan(t)
	var logs hookLogs

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil)
	pr, err := exec.begin(plan)
	assert.NilError(t, err)
	assert.NilError(t, pr.runCreate(t.Context()))
	assert.NilError(t, pr.runStart(t.Context(), logs.listener))
	assert.DeepEqual(t, logs.got(), []string{"post-start ok"})
}

// TestPlanRunStartListenerReplacesTheConstructionTimeOne verifies that a
// listener handed to runStart takes over from the one the executor was built
// with: the hook logs go to the former only.
func TestPlanRunStartListenerReplacesTheConstructionTimeOne(t *testing.T) {
	svc, plan := postStartHookPlan(t)
	var constructed, given hookLogs

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), constructed.listener)
	pr, err := exec.begin(plan)
	assert.NilError(t, err)
	assert.NilError(t, pr.runCreate(t.Context()))
	assert.NilError(t, pr.runStart(t.Context(), given.listener))
	assert.DeepEqual(t, given.got(), []string{"post-start ok"})
	assert.Equal(t, len(constructed.got()), 0)
}

// TestExecutePlanKeepsTheConstructionTimeListener verifies that run() -- which
// hands no listener to runStart -- still streams the Start phase's hook logs
// to the one the executor was built with, and that a nil runStart listener
// never drops it.
func TestExecutePlanKeepsTheConstructionTimeListener(t *testing.T) {
	svc, plan := postStartHookPlan(t)
	var logs hookLogs

	exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), logs.listener)
	assert.NilError(t, exec.run(t.Context(), plan))
	assert.DeepEqual(t, logs.got(), []string{"post-start ok"})
}

// TestPlanRunRefusesAnIllegalPhaseSequence verifies the guards that keep the
// Create-phase barrier intact for a caller driving the phases itself: the
// Start phase never runs before the Create phase, after a failed one, or
// twice; and a phase is never run twice either. Each mocked call is expected
// exactly as often as the sequence makes it, and none when it makes none: a
// call beyond that, or a missing one, fails the test.
func TestPlanRunRefusesAnIllegalPhaseSequence(t *testing.T) {
	// newRun plans a network create followed by a container start; createErr
	// is what the network create returns, creates and starts how many
	// NetworkCreate and ContainerStart calls the sequence under test makes.
	newRun := func(t *testing.T, createErr error, creates, starts int) *planRun {
		svc, apiClient := newTestService(t)
		nw := types.NetworkConfig{Name: "test_default"}
		if creates > 0 {
			result := client.NetworkCreateResult{ID: "net-id"}
			if createErr != nil {
				result = client.NetworkCreateResult{}
			}
			apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
				Return(result, createErr).Times(creates)
		}
		if starts > 0 {
			apiClient.EXPECT().ContainerStart(gomock.Any(), "db-id", gomock.Any()).
				Return(client.ContainerStartResult{}, nil).Times(starts)
		}
		plan := &Plan{}
		network := plan.addNode(Operation{Type: OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")
		plan.addNode(Operation{
			Type:       OpStartContainer,
			ResourceID: "service:db:1",
			Container:  &container.Summary{ID: "db-id", Names: []string{"/test-db-1"}},
		}, "", network).Phase = PhaseStart
		pr, err := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil).begin(plan)
		assert.NilError(t, err)
		return pr
	}

	t.Run("start before create", func(t *testing.T) {
		pr := newRun(t, nil, 0, 0)
		assert.ErrorContains(t, pr.runStart(t.Context(), nil), "before the create phase succeeded")
	})
	t.Run("start after a failed create", func(t *testing.T) {
		pr := newRun(t, errors.New("boom"), 1, 0)
		assert.ErrorContains(t, pr.runCreate(t.Context()), "boom")
		assert.ErrorContains(t, pr.runStart(t.Context(), nil), "before the create phase succeeded")
	})
	t.Run("create twice", func(t *testing.T) {
		pr := newRun(t, nil, 1, 0)
		assert.NilError(t, pr.runCreate(t.Context()))
		assert.ErrorContains(t, pr.runCreate(t.Context()), "already ran")
	})
	t.Run("create again after a failed create", func(t *testing.T) {
		pr := newRun(t, errors.New("boom"), 1, 0)
		assert.ErrorContains(t, pr.runCreate(t.Context()), "boom")
		assert.ErrorContains(t, pr.runCreate(t.Context()), "already ran")
	})
	t.Run("start twice", func(t *testing.T) {
		pr := newRun(t, nil, 1, 1)
		assert.NilError(t, pr.runCreate(t.Context()))
		assert.NilError(t, pr.runStart(t.Context(), nil))
		assert.ErrorContains(t, pr.runStart(t.Context(), nil), "already ran")
	})
}

// TestPlanRunOfAnEmptyOrSinglePhasePlan verifies what
// begin documents: an empty plan, or one with a single phase, gives a planRun
// whose phases run without error and dispatch only the nodes the plan has.
func TestPlanRunOfAnEmptyOrSinglePhasePlan(t *testing.T) {
	t.Run("empty plan", func(t *testing.T) {
		svc, _ := newTestService(t) // any API call fails the test
		pr, err := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil).begin(&Plan{})
		assert.NilError(t, err)
		assert.NilError(t, pr.runCreate(t.Context()))
		assert.NilError(t, pr.runStart(t.Context(), nil))
	})
	t.Run("create phase only", func(t *testing.T) {
		svc, apiClient := newTestService(t) // no ContainerStart expectation: any start fails the test
		apiClient.EXPECT().NetworkCreate(gomock.Any(), "test_default", gomock.Any()).
			Return(client.NetworkCreateResult{ID: "net-id"}, nil).Times(1)
		nw := types.NetworkConfig{Name: "test_default"}
		plan := &Plan{}
		plan.addNode(Operation{Type: OpCreateNetwork, ResourceID: "network:default", Name: nw.Name, Network: &nw}, "")
		pr, err := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"), nil).begin(plan)
		assert.NilError(t, err)
		assert.NilError(t, pr.runCreate(t.Context()))
		assert.NilError(t, pr.runStart(t.Context(), nil))
	})
}
