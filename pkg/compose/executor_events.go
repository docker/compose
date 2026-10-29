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
	"slices"
	"sync"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// groupTracker manages event emission for grouped nodes (e.g. recreate, or a
// replica's start chain). The first node starting emits Working, the last
// finishing emits Done.
type groupTracker struct {
	mu     sync.Mutex
	groups map[string]*groupState
	exec   *planExecutor // resolves the event name of a not-yet-materialized container
}

// groupKind picks the Working/Done status text for a group: distinct
// composite operations get distinct progressions on the event contract
// (recreate: "Recreate"/"Recreated"; a replica's start chain — pre_start,
// start, post_start folded into one line — the same Starting/Started
// progression a plain start reports).
type groupKind int

const (
	groupRecreate groupKind = iota
	groupStart
)

func groupKindOf(t planning.OperationType) groupKind {
	switch t {
	case planning.OpRunPreStart, planning.OpStartContainer, planning.OpRunPostStart:
		return groupStart
	default:
		return groupRecreate
	}
}

func (k groupKind) workingText() string {
	if k == groupStart {
		return api.StatusStarting
	}
	return "Recreate"
}

func (k groupKind) doneText() string {
	if k == groupStart {
		return api.StatusStarted
	}
	return "Recreated"
}

type groupState struct {
	kind      groupKind
	eventName string // e.g. "Container myproject-web-1"; resolved lazily for a start group (see groupEventName)
	total     int    // total nodes in this group
	started   int    // nodes that have started
	done      int    // nodes that have completed
	working   bool   // the group's Working event has been emitted
	finished  bool   // the group's final event (Done or Error) has been emitted
}

func (exec *planExecutor) buildGroupTracker(plan *planning.Plan) *groupTracker {
	gt := &groupTracker{groups: map[string]*groupState{}, exec: exec}
	for _, node := range plan.Nodes {
		if node.Group == "" {
			continue
		}
		gs, ok := gt.groups[node.Group]
		if !ok {
			gs = &groupState{kind: groupKindOf(node.Operation.Type)}
			gt.groups[node.Group] = gs
		}
		gs.total++
		// Pick the event name from a node that has the existing container
		// reference. A start group's first node may still be a plan-created
		// replica with no Summary yet — its name resolves lazily, once
		// execution reaches it (see onNodeStart).
		if gs.eventName == "" && node.Operation.Container != nil {
			gs.eventName = getContainerProgressName(*node.Operation.Container)
		}
	}
	return gt
}

// groupEventName names a group from its first executing node, for the case
// buildGroupTracker could not resolve statically: a start-phase node
// targeting a replica the plan itself creates. By the time this node starts,
// the CreateContainer node it depends on has already run and published its
// result (see resolveContainerID) — the DAG dependency guarantees it.
func (exec *planExecutor) groupEventName(op planning.Operation) string {
	if op.Container != nil {
		return getContainerProgressName(*op.Container)
	}
	if name := exec.pctx.get(op.CreateNodeID).ContainerName; name != "" {
		return "Container " + name
	}
	return op.ResourceID
}

// onNodeStart tells whether node may go on: not when its group was given up by
// another node of the group while this one was getting a slot. A Working event
// now would follow the group's final one and leave the row on it, and so would
// whatever the node then fails with.
func (gt *groupTracker) onNodeStart(node *planning.PlanNode, events api.EventProcessor) bool {
	if node.Group == "" {
		// Ungrouped: emit individual event
		emitStartEvent(node, events)
		return true
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	if gs.finished {
		return false
	}
	if gs.eventName == "" {
		gs.eventName = gt.exec.groupEventName(node.Operation)
	}
	gs.started++
	if gs.triggersWorking(node.Operation.Type) {
		gs.working = true
		events.On(progress.New(gs.eventName, api.Working, gs.kind.workingText()))
	}
	return true
}

// triggersWorking reports whether this node starting should fire the group's
// Working event. A recreate group fires on its first node; a start group
// fires specifically on OpStartContainer — pre_start hooks, when planned,
// run silently before it: no Starting event is surfaced until the
// ContainerStart call itself begins.
func (gs *groupState) triggersWorking(t planning.OperationType) bool {
	if gs.kind == groupStart {
		return t == planning.OpStartContainer
	}
	return gs.started == 1
}

func (gt *groupTracker) onNodeDone(node *planning.PlanNode, events api.EventProcessor) {
	if node.Group == "" {
		emitDoneEvent(node, events)
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	gs.done++
	if gs.done == gs.total && !gs.finished {
		gs.finished = true
		events.On(progress.New(gs.eventName, api.Done, gs.kind.doneText()))
	}
}

func (gt *groupTracker) onNodeError(node *planning.PlanNode, events api.EventProcessor, err error) {
	if node.Group == "" {
		emitErrorEvent(node, events, err)
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	if !gs.working {
		// This group's failing node never triggered Working itself (e.g.
		// OpRunPreStart, which runs silently by design -- see
		// triggersWorking). Emit it now so the group's progression stays
		// well-formed (Working always precedes Done/Error) instead of a bare
		// Error a progress consumer never saw the resource transition into.
		gs.working = true
		events.On(progress.New(gs.eventName, api.Working, gs.kind.workingText()))
	}
	gs.finished = true
	events.On(api.Resource{
		ID:     gs.eventName,
		Status: api.Error,
		Text:   err.Error(),
	})
}

// onNodeAborted gives a group a final event when one of its nodes was given up
// because another node of the phase failed. Without it the row stays at its
// last status for good: a replica recreated, then never started, still reads
// "Recreated", as if nothing was left to do. It is a no-op once the group has
// its final event, so the failing node's own Error (which also ends the
// nodes depending on it) is never followed by a second one.
func (gt *groupTracker) onNodeAborted(node *planning.PlanNode, events api.EventProcessor, reason string) {
	if node.Group == "" {
		return
	}
	gt.mu.Lock()
	defer gt.mu.Unlock()
	gs := gt.groups[node.Group]
	if gs.finished {
		return
	}
	gs.finished = true
	if gs.eventName == "" {
		gs.eventName = gt.exec.groupEventName(node.Operation)
	}
	if !gs.working {
		reason = "not started: " + reason
	}
	events.On(api.Resource{
		ID:     gs.eventName,
		Status: api.Error,
		Text:   reason,
	})
}

// resourceName is the name a node's resource goes by in the progress output.
func (gt *groupTracker) resourceName(node *planning.PlanNode) string {
	if node.Group != "" {
		gt.mu.Lock()
		name := gt.groups[node.Group].eventName
		gt.mu.Unlock()
		if name != "" {
			return name
		}
	}
	if node.Operation.Container != nil {
		return getContainerProgressName(*node.Operation.Container)
	}
	return node.Operation.ResourceID
}

// unmetCondition names the depends_on condition a wait node is bound to.
func unmetCondition(op planning.Operation) string {
	return fmt.Sprintf("condition %q of service %q", op.Condition, op.Name)
}

// interruption says what ended the phase: the resource whose failure
// interrupted it or, for the failure of a wait node, which has no resource of
// its own to name, the depends_on condition that was not met.
func (exec *planExecutor) interruption(rs *runState, failed *planning.PlanNode) string {
	if failed.Operation.Type == planning.OpWaitCondition {
		return "interrupted by the unmet " + unmetCondition(failed.Operation)
	}
	return "interrupted by the failure of " + rs.groups.resourceName(failed)
}

// unmetPrerequisite returns the wait node, among the prerequisites of node
// (transitively: a start is held through its pre_start, a post_start through
// its start), that has not succeeded, failed itself being preferred: which of
// the nodes of a chain reports first must not change what is said.
func unmetPrerequisite(node, failed *planning.PlanNode, succeeded func(int) bool) *planning.PlanNode {
	var first *planning.PlanNode
	seen := map[int]bool{}
	var walk func(n *planning.PlanNode)
	walk = func(n *planning.PlanNode) {
		for _, dep := range n.DependsOn {
			if seen[dep.ID] || succeeded(dep.ID) {
				continue
			}
			seen[dep.ID] = true
			if dep.Operation.Type == planning.OpWaitCondition && first == nil {
				first = dep
			}
			walk(dep)
		}
	}
	walk(node)
	if failed != nil && seen[failed.ID] && failed.Operation.Type == planning.OpWaitCondition {
		return failed
	}
	return first
}

// abortReason tells why node was given up after the failure of another node
// of its phase: the depends_on condition it was still waiting for, if that is
// what it was held by, and what ended the phase. A node held by the very wait
// that failed was not interrupted: its dependency did not meet the condition.
func (exec *planExecutor) abortReason(node *planning.PlanNode, rs *runState, failed *planning.PlanNode) string {
	reason := exec.interruption(rs, failed)
	if held := unmetPrerequisite(node, failed, rs.succeeded); held != nil {
		if held == failed {
			return unmetCondition(held.Operation) + " not met"
		}
		return fmt.Sprintf("%s not reached: %s", unmetCondition(held.Operation), reason)
	}
	if node.Operation.Type == planning.OpRunPostStart && slices.ContainsFunc(node.DependsOn, func(dep *planning.PlanNode) bool {
		return dep.Operation.Type == planning.OpStartContainer && rs.succeeded(dep.ID)
	}) {
		// the container is started by then: only its hooks were cut short
		return "started, post_start hooks " + reason
	}
	return reason
}

// reportAborted reports a node that did not run (or did not complete) because
// the start phase was canceled by the failure of another node, so that the row
// of the resource it was about to change ends on something other than its last
// status. A phase canceled by its caller (Ctrl-C) reports nothing, as before,
// and so does the create phase, whose failure never was reported on the nodes
// it gives up.
func (exec *planExecutor) reportAborted(node *planning.PlanNode, rs *runState, phase *phaseCancel) {
	if node.Phase != planning.PhaseStart || node.Operation.Type == planning.OpWaitCondition {
		return
	}
	failed := phase.failed.Load()
	if failed == nil || failed == node {
		return
	}
	rs.groups.onNodeAborted(node, rs.events, exec.abortReason(node, rs, failed))
}

// interruptedWait is the failure of a wait node left by the failure of
// another node of its phase: the depends_on condition was not reached. The
// rows of the containers it was waiting on, left on "Waiting", are closed with
// the same reason. A phase not canceled by one of its own nodes is not an
// interruption to report: the wait ends as it always did, with cause.
func (exec *planExecutor) interruptedWait(node *planning.PlanNode, rs *runState, phase *phaseCancel, cause error) error {
	failed := phase.failed.Load()
	if failed == nil {
		return cause
	}
	op := node.Operation
	exec.containersMu.Lock()
	waitingFor := exec.containersByService[op.Name].filter(isNotOneOff, isNotHookContainer)
	exec.containersMu.Unlock()
	reason := fmt.Sprintf("%s not reached: %s", unmetCondition(op), exec.interruption(rs, failed))
	rs.events.On(containerReasonEvents(waitingFor, progress.Skipped, reason)...)
	return fmt.Errorf("%s: %w", reason, context.Canceled)
}

// emitStartEvent emits the appropriate Working event for an ungrouped node.
//
//nolint:dupl // mirrors emitDoneEvent: keeping the Working and Done events side by side keeps their pairing visible
func emitStartEvent(node *planning.PlanNode, events api.EventProcessor) {
	op := node.Operation
	switch op.Type {
	case planning.OpCreateContainer:
		events.On(progress.Creating("Container " + op.Name))
	case planning.OpStartContainer:
		name := getContainerProgressName(*op.Container)
		events.On(progress.New(name, api.Working, api.StatusStarting))
	case planning.OpStopContainer:
		events.On(progress.Stopping(getContainerProgressName(*op.Container)))
	case planning.OpRemoveContainer:
		events.On(progress.Removing(getContainerProgressName(*op.Container)))
	case planning.OpCreateNetwork:
		events.On(progress.Creating("Network " + op.Name))
	case planning.OpRemoveNetwork:
		events.On(progress.Removing("Network " + op.Name))
	case planning.OpCreateVolume:
		events.On(progress.Creating("Volume " + op.Name))
	case planning.OpRemoveVolume:
		events.On(progress.Removing("Volume " + op.Name))
	}
}

// emitDoneEvent emits the appropriate Done event for an ungrouped node.
//
//nolint:dupl // mirrors emitStartEvent, see there
func emitDoneEvent(node *planning.PlanNode, events api.EventProcessor) {
	op := node.Operation
	switch op.Type {
	case planning.OpCreateContainer:
		events.On(progress.Created("Container " + op.Name))
	case planning.OpStartContainer:
		name := getContainerProgressName(*op.Container)
		events.On(progress.New(name, api.Done, api.StatusStarted))
	case planning.OpStopContainer:
		events.On(progress.Stopped(getContainerProgressName(*op.Container)))
	case planning.OpRemoveContainer:
		events.On(progress.Removed(getContainerProgressName(*op.Container)))
	case planning.OpCreateNetwork:
		events.On(progress.Created("Network " + op.Name))
	case planning.OpRemoveNetwork:
		events.On(progress.Removed("Network " + op.Name))
	case planning.OpCreateVolume:
		events.On(progress.Created("Volume " + op.Name))
	case planning.OpRemoveVolume:
		events.On(progress.Removed("Volume " + op.Name))
	}
}

// emitErrorEvent emits an error event for an ungrouped node.
func emitErrorEvent(node *planning.PlanNode, events api.EventProcessor, err error) {
	op := node.Operation
	if op.Type == planning.OpWaitCondition {
		// execWaitCondition already reported this failure as one event per
		// container of the dependency it was waiting on (see
		// waitDependency/checkDependency*), exactly like waitDependencies
		// does today. A second, generic event on "wait:..." here would be a
		// confusing duplicate with no matching resource.
		return
	}
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
