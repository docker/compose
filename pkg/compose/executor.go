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
	"fmt"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/docker/compose/v5/pkg/api"
)

// planExecutor executes a reconciliation Plan by walking the DAG and performing
// each atomic operation via the Docker API. It carries no decision logic — all
// decisions were made by the reconciler when building the plan.
type planExecutor struct {
	compose *composeService
	project *types.Project
	pctx    *reconciliationContext

	// listener streams pre_start/post_start hook logs, exactly like the
	// imperative start path. nil until an attached caller wires one in (the
	// interactive-up convergence, a later lot of #14081) — every caller today
	// passes nil, so hook execution stays silent, matching today's detached
	// behavior.
	listener api.ContainerEventListener

	// containersByService is a live view used to resolve service references
	// (network_mode: service:x, volumes_from, ipc, pid) without a daemon
	// round-trip per create.
	containersMu        sync.Mutex
	containersByService map[string]Containers

	// waitTimeout, when non-zero, bounds OpWaitCondition nodes only (see
	// executeNode) -- not the rest of the plan, and with its OWN fresh
	// window per wait node rather than a single shared deadline counted
	// down from before the plan started. up -d --wait-timeout must cap how
	// long a dependency's health/completion condition is waited on, the
	// same thing start()'s own per-dependency WaitTimeout already does
	// today -- without also capping unrelated create/start/hook work, and
	// without letting that unrelated work's duration eat into (or exhaust)
	// a wait's own budget (docker/compose#14290 review).
	waitTimeout time.Duration
}

// reconciliationContext holds results produced by completed nodes so that downstream
// nodes can reference them (e.g. a RenameContainer node needs the container ID
// created by a prior CreateContainer node).
type reconciliationContext struct {
	mu      sync.Mutex
	results map[int]operationResult
}

type operationResult struct {
	ContainerID   string
	ContainerName string
}

func (pc *reconciliationContext) set(nodeID int, r operationResult) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.results[nodeID] = r
}

func (pc *reconciliationContext) get(nodeID int) operationResult {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.results[nodeID]
}

// executePlan walks the plan DAG, executing nodes in parallel where possible
// while respecting dependency edges. It emits progress events and handles
// group-based event aggregation for composite operations like recreate.
//
// No caller threads a hook-log listener through here yet: create() (its only
// caller) plans no start-phase operations today. newPlanExecutor takes one
// directly for that reason — it is what the interactive-up convergence (a
// later lot of #14081) will call once it needs to stream pre_start/post_start
// hook logs into an attached session.
func (s *composeService) executePlan(ctx context.Context, project *types.Project, observed *ObservedState, plan *Plan) error {
	return s.newPlanExecutor(project, observed, nil).run(ctx, plan)
}

// newPlanExecutor constructs a planExecutor seeded from the observed state.
// Split out from executePlan so tests can inspect the executor's live state
// (e.g. the containersByService cache) after running a plan.
func (s *composeService) newPlanExecutor(project *types.Project, observed *ObservedState, listener api.ContainerEventListener) *planExecutor {
	return &planExecutor{
		compose:             s,
		project:             project,
		listener:            listener,
		pctx:                &reconciliationContext{results: map[int]operationResult{}},
		containersByService: observed.containersByService(),
	}
}

// run walks the plan DAG, executing nodes in parallel where possible while
// respecting dependency edges. Emits progress events and handles group-based
// event aggregation for composite operations like recreate.
func (exec *planExecutor) run(ctx context.Context, plan *Plan) error {
	if plan.IsEmpty() {
		return nil
	}

	// Build a done-channel per node so dependents can wait
	done := make(map[int]chan struct{}, len(plan.Nodes))
	for _, node := range plan.Nodes {
		done[node.ID] = make(chan struct{})
	}

	// failures records the terminal error of every node that didn't succeed,
	// keyed by node ID. A dependent unblocked from <-done[dep.ID] consults it
	// to tell "dependency failed for a genuine reason" apart from "dependency
	// succeeded" -- ctx.Err() alone can't: close(done[dep.ID]) happens inside
	// the failing node's own goroutine, strictly before errgroup's cancel()
	// fires on that goroutine's returned error (cancel() only runs after it
	// returns), so a dependent can observe ctx.Err() == nil for a brief
	// window even though its dependency just failed. Without this, that
	// dependent runs anyway and spuriously emits its own group Working/
	// Starting event right after the dependency's Error already went out
	// (see TestExecutePlanFailedPreStartGatesStart).
	var failuresMu sync.Mutex
	failures := make(map[int]error, len(plan.Nodes))
	recordFailure := func(id int, err error) {
		failuresMu.Lock()
		failures[id] = err
		failuresMu.Unlock()
	}
	failedDependency := func(deps []*PlanNode) error {
		failuresMu.Lock()
		defer failuresMu.Unlock()
		for _, dep := range deps {
			if err := failures[dep.ID]; err != nil {
				return err
			}
		}
		return nil
	}

	// Track group event state: first node emits Working, last emits Done
	groups := exec.buildGroupTracker(plan)
	events := exec.compose.events

	rs := &runState{
		done:             done,
		recordFailure:    recordFailure,
		failedDependency: failedDependency,
		groups:           groups,
		events:           events,
		limiter:          newOptionalLimiter(exec.compose.maxConcurrency),
	}

	var createNodes, startNodes []*PlanNode
	for _, node := range plan.Nodes {
		if node.Phase == PhaseStart {
			startNodes = append(startNodes, node)
		} else {
			createNodes = append(createNodes, node)
		}
	}

	// A Create-phase node depending on a Start-phase node would deadlock
	// forever: runPhase(createNodes, ...) only closes rs.done for nodes in
	// createNodes, so runNode's `<-rs.done[dep.ID]` would block on a channel
	// nothing ever closes, since the Start phase isn't even dispatched until
	// the Create phase this node is blocking returns. Nothing in the
	// reconciler should ever produce this edge, but failing fast with a
	// clear error beats a silent hang if it ever does.
	for _, node := range createNodes {
		for _, dep := range node.DependsOn {
			if dep.Phase == PhaseStart {
				return fmt.Errorf("invalid plan: create-phase node %d depends on start-phase node %d", node.ID, dep.ID)
			}
		}
	}

	// The Create phase is a hard barrier: no Start-phase node is even
	// dispatched until every Create-phase node has succeeded. This isn't
	// just about not canceling unrelated in-flight Create work on a Start
	// failure (phaseCancel already keeps that scoped per phase, see its own
	// doc comment) -- it's that a Start-phase node whose own DependsOn
	// edges are all satisfied has no reason to wait for a COMPLETELY
	// UNRELATED Create-phase node elsewhere in the plan, so without this
	// barrier a service could already be starting while a sibling it has no
	// relationship with is still being created, or fails. The old
	// create()-then-start() sequence never allowed that: the whole create
	// phase, project-wide, always finished (or failed, with start() never
	// invoked at all) before any dependency wait, or anything else in the
	// start phase, even began. Interactive up's attach/printer session
	// depends on this too: it takes exclusive hold of the terminal for the
	// create phase's own progress display, handing it over to continuous
	// container log streaming only once the create phase is entirely done
	// -- a Start-phase node's log output interleaving with Create's
	// progress bars has nowhere consistent to go.
	if err := exec.runPhase(createNodes, rs, newPhaseCancel(ctx)); err != nil {
		return err
	}
	return exec.runPhase(startNodes, rs, newPhaseCancel(ctx))
}

// runPhase dispatches every node in nodes concurrently (respecting their own
// DependsOn edges via rs.done, same as a single-phase plan always did) and
// waits for them all to finish. phase is this call's own cancellation scope:
// a node failing here cancels phase.ctx for its siblings in THIS call only,
// never a concurrently-running call for a different phase (not that there
// is one today -- run() calls this sequentially -- but runNode has no way
// to tell, so this still matters if that ever changes). See phaseCancel's
// doc comment for why a node's failure goes through phase.fail instead of
// errgroup.Group's own first-error-wins race.
func (exec *planExecutor) runPhase(nodes []*PlanNode, rs *runState, phase *phaseCancel) error {
	defer phase.cancel()
	eg := errgroup.Group{}
	for _, node := range nodes {
		eg.Go(func() error {
			return exec.runNode(node, rs, phase)
		})
	}
	err := eg.Wait()
	if phase.err != nil {
		return phase.err
	}
	return err
}

// phaseCancel bundles one phase's cancellation context with the bookkeeping
// needed to make exactly one node's failure authoritative for it: fail is
// called by every node in this phase that errors out, but its body -- the
// event emission and the actual ctx cancellation -- runs for only the first
// caller, via once. That single gate point fixes two related races a plain
// "if nodeCtx.Err() == nil" check can't: two genuinely concurrent failures
// in the same phase racing to emit onNodeError (the old errgroup.WithContext
// made this vanishingly unlikely by canceling ctx atomically with recording
// the error; splitting cancellation into our own explicit call reopened the
// window), and errgroup.Group's own first-error-wins race picking a
// cancellation casualty's bare context.Canceled over the actual originating
// error once cancellation starts unblocking siblings (see run()'s use of
// err, set here, instead of eg.Wait()'s own return value).
type phaseCancel struct {
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func newPhaseCancel(parent context.Context) *phaseCancel {
	p := &phaseCancel{}
	p.ctx, p.cancel = context.WithCancel(parent)
	return p
}

// fail registers node's failure as this phase's cause, for the first caller
// only: it emits the group error event and cancels the phase's context. Any
// later caller's err is silently dropped -- by the time it could run, this
// phase is already canceled, so it is never anything but a cancellation
// casualty of the first failure, not a new one of its own.
func (p *phaseCancel) fail(node *PlanNode, events api.EventProcessor, groups *groupTracker, err error) {
	p.once.Do(func() {
		p.err = err
		groups.onNodeError(node, events, err)
		p.cancel()
	})
}

// runState carries the per-run() state every node's goroutine shares: the
// done-channel per node, dependency-failure bookkeeping, group event
// tracking, and the concurrency limiter. Shared across both phases (built
// once in run(), before either runs) since done/failures must stay visible
// to a Start-phase node depending on a Create-phase one across the barrier
// between them. Split out of run() purely to keep runNode a plain method
// instead of a closure captured in a loop.
type runState struct {
	done             map[int]chan struct{}
	recordFailure    func(id int, err error)
	failedDependency func(deps []*PlanNode) error
	groups           *groupTracker
	events           api.EventProcessor
	limiter          *semaphore.Weighted
}

// runNode executes one plan node: waits for its dependencies, skips it if
// one of them failed, then dispatches it and records the outcome. phase is
// the cancellation scope of the runPhase call driving it -- see runPhase and
// phaseCancel's own doc comments.
func (exec *planExecutor) runNode(node *PlanNode, rs *runState, phase *phaseCancel) error {
	nodeCtx := phase.ctx

	// Every exit path below must close rs.done[node.ID] exactly once, or a
	// dependent blocks on it forever: deferred once here instead of before
	// each individual return, so a future added exit path can't reintroduce
	// the gap the original dependency-wait early-return had (no reachable
	// hang today -- every dependent in the same phase shares this same
	// nodeCtx, already canceled by the time it would matter -- but nothing
	// enforces that staying true as the plan DAG grows new shapes).
	defer close(rs.done[node.ID])

	// Wait for all dependencies
	for _, dep := range node.DependsOn {
		select {
		case <-rs.done[dep.ID]:
		case <-nodeCtx.Done():
			return nodeCtx.Err()
		}
	}

	// A dependency may have just failed without nodeCtx being canceled yet
	// (see run()'s failures comment) -- skip this node exactly like a
	// canceled one: no slot acquired, no event emitted, its own dependents
	// unblocked with the same failure recorded.
	if err := rs.failedDependency(node.DependsOn); err != nil {
		// Store the original error, not a wrapped one: this node's own
		// dependents look it up the same way, and wrapping it again at
		// every hop would compound "dependency failed: " prefixes down a
		// multi-node chain (create -> pre_start -> start -> post_start).
		rs.recordFailure(node.ID, err)
		return fmt.Errorf("dependency failed: %w", err)
	}

	if err := acquireSlot(nodeCtx, rs.limiter); err != nil {
		rs.recordFailure(node.ID, err)
		return err
	}
	defer releaseSlot(rs.limiter)

	// Emit group start event if this is the first node of a group
	rs.groups.onNodeStart(node, rs.events)

	err := exec.executeNode(nodeCtx, node)

	if err == nil {
		// Emit group done event if this is the last node of a group
		rs.groups.onNodeDone(node, rs.events)
		return nil
	}

	rs.recordFailure(node.ID, err)
	if nodeCtx.Err() == nil {
		// This node's own failure, not an inherited cancellation: a node
		// whose nodeCtx is already canceled failed because of that
		// cancellation, not a new failure of its own to propagate. phase.fail
		// itself gates on being the first such failure in this phase (see its
		// doc comment) -- this outer check only spares every later,
		// cancellation-casualty caller the cost of building the error-event
		// payload it would be dropped anyway.
		phase.fail(node, rs.events, rs.groups, err)
	}
	return err
}

// executeNode dispatches a single plan node to the appropriate API call.
func (exec *planExecutor) executeNode(ctx context.Context, node *PlanNode) error {
	op := node.Operation
	switch op.Type {
	case OpCreateNetwork:
		return exec.execCreateNetwork(ctx, op)
	case OpRemoveNetwork:
		return exec.execRemoveNetwork(ctx, op)
	case OpDisconnectNetwork:
		return exec.execDisconnectNetwork(ctx, op)
	case OpConnectNetwork:
		return exec.execConnectNetwork(ctx, op)
	case OpCreateVolume:
		return exec.execCreateVolume(ctx, op)
	case OpRemoveVolume:
		return exec.execRemoveVolume(ctx, op)
	case OpCreateContainer:
		return exec.execCreateContainer(ctx, node)
	case OpStartContainer:
		return exec.execStartContainer(ctx, op)
	case OpStopContainer:
		return exec.execStopContainer(ctx, op)
	case OpRemoveContainer:
		return exec.execRemoveContainer(ctx, op)
	case OpRenameContainer:
		return exec.execRenameContainer(ctx, node)
	case OpCreateHookContainer:
		return exec.execCreateHookContainer(ctx, node)
	case OpWaitCondition:
		origCtx := ctx
		if exec.waitTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, exec.waitTimeout)
			defer cancel()
		}
		return exec.execWaitCondition(ctx, origCtx, op)
	case OpRunPreStart:
		return exec.execRunPreStart(ctx, op)
	case OpRunPostStart:
		return exec.execRunPostStart(ctx, op)
	case OpRunProvider:
		return exec.compose.runPlugin(ctx, exec.project, *op.Service, "up")
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}
