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
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// planExecutor executes a reconciliation Plan by walking the DAG and performing
// each atomic operation via the Docker API. It carries no decision logic — all
// decisions were made by the reconciler when building the plan.
type planExecutor struct {
	compose *composeService
	project *types.Project
	pctx    *reconciliationContext
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
func (s *composeService) executePlan(ctx context.Context, project *types.Project, plan *Plan) error {
	if plan.IsEmpty() {
		return nil
	}

	exec := &planExecutor{
		compose: s,
		project: project,
		pctx:    &reconciliationContext{results: map[int]operationResult{}},
	}

	// Build a done-channel per node so dependents can wait
	done := make(map[int]chan struct{}, len(plan.Nodes))
	for _, node := range plan.Nodes {
		done[node.ID] = make(chan struct{})
	}

	// Track group event state: first node emits Working, last emits Done
	groups := exec.buildGroupTracker(plan)

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
			groups.onNodeStart(node, s.events)

			err := exec.executeNode(ctx, node)

			if err == nil {
				// Emit group done event if this is the last node of a group
				groups.onNodeDone(node, s.events)
			} else if ctx.Err() == nil {
				groups.onNodeError(node, s.events, err)
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

// --- Network operations ---

func (exec *planExecutor) execCreateNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.ensureNetwork(ctx, exec.project, op.Name, op.Network)
	return err
}

func (exec *planExecutor) execRemoveNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkRemove(ctx, op.Name, client.NetworkRemoveOptions{})
	return err
}

func (exec *planExecutor) execDisconnectNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkDisconnect(ctx, op.Name, client.NetworkDisconnectOptions{
		Container: op.Container.ID,
		Force:     true,
	})
	return err
}

func (exec *planExecutor) execConnectNetwork(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().NetworkConnect(ctx, op.Name, client.NetworkConnectOptions{
		Container: op.Container.ID,
	})
	return err
}

// --- Volume operations ---

func (exec *planExecutor) execCreateVolume(ctx context.Context, op Operation) error {
	return exec.compose.createVolume(ctx, *op.Volume)
}

func (exec *planExecutor) execRemoveVolume(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().VolumeRemove(ctx, op.Name, client.VolumeRemoveOptions{Force: true})
	return err
}

// --- Container operations ---

func (exec *planExecutor) execCreateContainer(ctx context.Context, node *PlanNode) error {
	op := node.Operation
	service := *op.Service

	// Resolve service references (network_mode, ipc, pid, volumes_from) to actual
	// container IDs. This must happen at execution time because the referenced
	// containers may have just been created by earlier plan nodes.
	observedState, err := exec.compose.getContainers(ctx, exec.project.Name, oneOffExclude, true)
	if err != nil {
		return err
	}
	conv := newConvergence(exec.project.ServiceNames(), observedState, nil, nil, exec.compose)
	if err := conv.resolveServiceReferences(&service); err != nil {
		return err
	}

	labels := mergeLabels(service.Labels, service.CustomLabels)
	if op.Container != nil {
		// This is a recreate: add the replace label
		replacedName := op.Service.ContainerName
		if replacedName == "" {
			replacedName = fmt.Sprintf("%s%s%d", op.Service.Name, api.Separator, op.Number)
		}
		labels = labels.Add(api.ContainerReplaceLabel, replacedName)
	}

	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            labels,
	}
	ctr, err := exec.compose.createMobyContainer(ctx, exec.project, service, op.Name, op.Number, op.Inherited, opts)
	if err != nil {
		return err
	}

	exec.pctx.set(node.ID, operationResult{
		ContainerID:   ctr.ID,
		ContainerName: op.Name,
	})
	return nil
}

func (exec *planExecutor) execStartContainer(ctx context.Context, op Operation) error {
	startMx.Lock()
	defer startMx.Unlock()
	_, err := exec.compose.apiClient().ContainerStart(ctx, op.Container.ID, client.ContainerStartOptions{})
	return err
}

func (exec *planExecutor) execStopContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerStop(ctx, op.Container.ID, client.ContainerStopOptions{
		Timeout: utils.DurationSecondToInt(op.Timeout),
	})
	return err
}

func (exec *planExecutor) execRemoveContainer(ctx context.Context, op Operation) error {
	_, err := exec.compose.apiClient().ContainerRemove(ctx, op.Container.ID, client.ContainerRemoveOptions{Force: true})
	return err
}

func (exec *planExecutor) execRenameContainer(ctx context.Context, node *PlanNode) error {
	// Find the CreateContainer node in our dependencies to get the created container ID
	var createdID string
	for _, dep := range node.DependsOn {
		r := exec.pctx.get(dep.ID)
		if r.ContainerID != "" {
			createdID = r.ContainerID
			break
		}
		// Also check transitive: the create node may not be a direct dep
		// Walk up the group chain
	}
	// Fallback: walk all results in the group
	if createdID == "" {
		for _, n := range node.DependsOn {
			createdID = exec.findCreatedIDInChain(n)
			if createdID != "" {
				break
			}
		}
	}
	if createdID == "" {
		return fmt.Errorf("rename: could not find created container ID for node #%d", node.ID)
	}

	_, err := exec.compose.apiClient().ContainerRename(ctx, createdID, client.ContainerRenameOptions{
		NewName: node.Operation.Name,
	})
	return err
}

// findCreatedIDInChain walks the dependency chain to find a CreateContainer result.
func (exec *planExecutor) findCreatedIDInChain(node *PlanNode) string {
	r := exec.pctx.get(node.ID)
	if r.ContainerID != "" {
		return r.ContainerID
	}
	for _, dep := range node.DependsOn {
		if id := exec.findCreatedIDInChain(dep); id != "" {
			return id
		}
	}
	return ""
}

// --- Group event tracking ---

// groupTracker manages event emission for grouped nodes (e.g. recreate).
// The first node starting emits Working, the last finishing emits Done.
type groupTracker struct {
	mu     sync.Mutex
	groups map[string]*groupState
}

type groupState struct {
	eventName string // e.g. "Container myproject-web-1"
	total     int    // total nodes in this group
	started   int    // nodes that have started
	done      int    // nodes that have completed
}

func (exec *planExecutor) buildGroupTracker(plan *Plan) *groupTracker {
	gt := &groupTracker{groups: map[string]*groupState{}}
	for _, node := range plan.Nodes {
		if node.Group == "" {
			continue
		}
		if _, ok := gt.groups[node.Group]; !ok {
			gt.groups[node.Group] = &groupState{}
		}
		gt.groups[node.Group].total++
		// Pick the event name from a node that has the existing container reference
		if gt.groups[node.Group].eventName == "" && node.Operation.Container != nil {
			gt.groups[node.Group].eventName = getContainerProgressName(*node.Operation.Container)
		}
	}
	// Fallback for groups where no node had a Container (shouldn't happen for recreate)
	for name, gs := range gt.groups {
		if gs.eventName == "" {
			gs.eventName = name
		}
	}
	return gt
}

func (gt *groupTracker) onNodeStart(node *PlanNode, events api.EventProcessor) {
	if node.Group == "" {
		// Ungrouped: emit individual event
		emitStartEvent(node, events)
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	gs.started++
	if gs.started == 1 {
		events.On(newEvent(gs.eventName, api.Working, "Recreate"))
	}
}

func (gt *groupTracker) onNodeDone(node *PlanNode, events api.EventProcessor) {
	if node.Group == "" {
		emitDoneEvent(node, events)
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	gs.done++
	if gs.done == gs.total {
		events.On(newEvent(gs.eventName, api.Done, "Recreated"))
	}
}

func (gt *groupTracker) onNodeError(node *PlanNode, events api.EventProcessor, err error) {
	if node.Group == "" {
		emitErrorEvent(node, events, err)
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	events.On(api.Resource{
		ID:     gs.eventName,
		Status: api.Error,
		Text:   err.Error(),
	})
}

// emitStartEvent emits the appropriate Working event for an ungrouped node.
func emitStartEvent(node *PlanNode, events api.EventProcessor) {
	op := node.Operation
	switch op.Type {
	case OpCreateContainer:
		events.On(creatingEvent("Container " + op.Name))
	case OpStartContainer:
		name := getContainerProgressName(*op.Container)
		events.On(newEvent(name, api.Working, api.StatusStarting))
	case OpStopContainer:
		events.On(stoppingEvent(getContainerProgressName(*op.Container)))
	case OpRemoveContainer:
		events.On(removingEvent(getContainerProgressName(*op.Container)))
	case OpCreateNetwork:
		events.On(creatingEvent("Network " + op.Name))
	case OpRemoveNetwork:
		events.On(removingEvent("Network " + op.Name))
	case OpCreateVolume:
		events.On(creatingEvent("Volume " + op.Name))
	case OpRemoveVolume:
		events.On(removingEvent("Volume " + op.Name))
	}
}

// emitDoneEvent emits the appropriate Done event for an ungrouped node.
func emitDoneEvent(node *PlanNode, events api.EventProcessor) {
	op := node.Operation
	switch op.Type {
	case OpCreateContainer:
		events.On(createdEvent("Container " + op.Name))
	case OpStartContainer:
		name := getContainerProgressName(*op.Container)
		events.On(newEvent(name, api.Done, api.StatusStarted))
	case OpStopContainer:
		events.On(stoppedEvent(getContainerProgressName(*op.Container)))
	case OpRemoveContainer:
		events.On(removedEvent(getContainerProgressName(*op.Container)))
	case OpCreateNetwork:
		events.On(createdEvent("Network " + op.Name))
	case OpRemoveNetwork:
		events.On(removedEvent("Network " + op.Name))
	case OpCreateVolume:
		events.On(createdEvent("Volume " + op.Name))
	case OpRemoveVolume:
		events.On(removedEvent("Volume " + op.Name))
	}
}

// emitErrorEvent emits an error event for an ungrouped node.
func emitErrorEvent(node *PlanNode, events api.EventProcessor, err error) {
	op := node.Operation
	var id string
	switch {
	case op.Container != nil:
		id = getContainerProgressName(*op.Container)
	default:
		id = op.ResourceID
	}
	events.On(api.Resource{
		ID:     id,
		Status: api.Error,
		Text:   err.Error(),
	})
}

// getContainerProgressName returns the display name for a container in progress output.
// Re-exported from convergence.go for use by the executor.
var _ = getContainerProgressName // ensure it stays accessible
