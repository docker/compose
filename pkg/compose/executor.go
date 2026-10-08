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

	// Every node's goroutine is dispatched unconditionally, so one waiting on
	// a dependency never occupies a concurrency slot -- only the actual
	// executeNode call does, via the semaphore below. This also means
	// deadlock-freedom no longer depends on plan.Nodes staying topologically
	// sorted: a goroutine blocked on <-done[dep.ID] holds no slot for a
	// still-pending dependency to be starved on.
	eg, ctx := errgroup.WithContext(ctx)
	limiter := newOptionalLimiter(exec.compose.maxConcurrency)
	for _, node := range plan.Nodes {
		eg.Go(func() error {
			// Wait for all dependencies
			for _, dep := range node.DependsOn {
				select {
				case <-done[dep.ID]:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			// A dependency may have just failed without ctx being canceled
			// yet (see failures' comment above) -- skip this node exactly
			// like a canceled one: no slot acquired, no event emitted, its
			// own dependents unblocked with the same failure recorded.
			if err := failedDependency(node.DependsOn); err != nil {
				// Store the original error, not a wrapped one: this node's own
				// dependents look it up the same way, and wrapping it again at
				// every hop would compound "dependency failed: " prefixes down
				// a multi-node chain (create -> pre_start -> start -> post_start).
				recordFailure(node.ID, err)
				close(done[node.ID])
				return fmt.Errorf("dependency failed: %w", err)
			}

			release, slotErr := acquireNodeSlot(ctx, limiter, node)
			if slotErr != nil {
				recordFailure(node.ID, slotErr)
				close(done[node.ID])
				return slotErr
			}
			defer release()

			// Emit group start event if this is the first node of a group
			groups.onNodeStart(node, events)

			err := exec.executeNode(ctx, node)

			if err == nil {
				// Emit group done event if this is the last node of a group
				groups.onNodeDone(node, events)
			} else {
				recordFailure(node.ID, err)
				if ctx.Err() == nil {
					groups.onNodeError(node, events, err)
				}
			}

			close(done[node.ID])
			return err
		})
	}

	return eg.Wait()
}

// acquireNodeSlot takes node's --parallel slot and returns the function that
// releases it. An OpWaitCondition node holds none: it only polls the daemon on
// a ticker, so a slot held for the whole polling would be capacity lost for
// every unrelated node, and could starve the nodes it is waiting for. The
// latter needs a dependency that is already running, hence with no start
// node for the wait to depend on, whose health relies on a sibling the plan
// has yet to start: with a bounded --parallel the wait could take the slot
// that sibling's start needs and only end at its timeout. The imperative
// waitDependencies stays outside the cap for the same reason.
func acquireNodeSlot(ctx context.Context, limiter *semaphore.Weighted, node *PlanNode) (func(), error) {
	if node.Operation.Type == OpWaitCondition {
		return func() {}, nil
	}
	if err := acquireSlot(ctx, limiter); err != nil {
		return nil, err
	}
	return func() { releaseSlot(limiter) }, nil
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
		return exec.execWaitCondition(ctx, op)
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
