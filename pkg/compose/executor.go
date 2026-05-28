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
)

// planExecutor executes a reconciliation Plan by walking the DAG and performing
// each atomic operation via the Docker API. It carries no decision logic — all
// decisions were made by the reconciler when building the plan.
type planExecutor struct {
	compose *composeService
	project *types.Project
	pctx    *reconciliationContext

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
func (s *composeService) executePlan(ctx context.Context, project *types.Project, observed *ObservedState, plan *Plan) error {
	return s.newPlanExecutor(project, observed).run(ctx, plan)
}

// newPlanExecutor constructs a planExecutor seeded from the observed state.
// Split out from executePlan so tests can inspect the executor's live state
// (e.g. the containersByService cache) after running a plan.
func (s *composeService) newPlanExecutor(project *types.Project, observed *ObservedState) *planExecutor {
	return &planExecutor{
		compose:             s,
		project:             project,
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

	// Track group event state: first node emits Working, last emits Done
	groups := exec.buildGroupTracker(plan)
	events := exec.compose.events

	eg, ctx := errgroup.WithContext(ctx)
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

			// Emit group start event if this is the first node of a group
			groups.onNodeStart(node, events)

			err := exec.executeNode(ctx, node)

			if err == nil {
				// Emit group done event if this is the last node of a group
				groups.onNodeDone(node, events)
			} else if ctx.Err() == nil {
				groups.onNodeError(node, events, err)
			}

			close(done[node.ID])
			return err
		})
	}

	return eg.Wait()
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
	case OpRunProvider:
		return exec.compose.runPlugin(ctx, exec.project, *op.Service, "up")
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}
