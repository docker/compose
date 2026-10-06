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
	"sync"

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

func groupKindOf(t OperationType) groupKind {
	switch t {
	case OpRunPreStart, OpStartContainer, OpRunPostStart:
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
}

func (exec *planExecutor) buildGroupTracker(plan *Plan) *groupTracker {
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
func (exec *planExecutor) groupEventName(op Operation) string {
	if op.Container != nil {
		return getContainerProgressName(*op.Container)
	}
	if name := exec.pctx.get(op.CreateNodeID).ContainerName; name != "" {
		return "Container " + name
	}
	return op.ResourceID
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
	if gs.eventName == "" {
		gs.eventName = gt.exec.groupEventName(node.Operation)
	}
	gs.started++
	if gs.triggersWorking(node.Operation.Type) {
		gs.working = true
		events.On(newEvent(gs.eventName, api.Working, gs.kind.workingText()))
	}
}

// triggersWorking reports whether this node starting should fire the group's
// Working event. A recreate group fires on its first node; a start group
// fires specifically on OpStartContainer — pre_start hooks, when planned,
// run silently before it, matching the imperative engine's startService,
// which never surfaces a Starting event until the ContainerStart call itself
// begins.
func (gs *groupState) triggersWorking(t OperationType) bool {
	if gs.kind == groupStart {
		return t == OpStartContainer
	}
	return gs.started == 1
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
		events.On(newEvent(gs.eventName, api.Done, gs.kind.doneText()))
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
	if !gs.working {
		// This group's failing node never triggered Working itself (e.g.
		// OpRunPreStart, which runs silently by design -- see
		// triggersWorking). Emit it now so the group's progression stays
		// well-formed (Working always precedes Done/Error) instead of a bare
		// Error a progress consumer never saw the resource transition into.
		gs.working = true
		events.On(newEvent(gs.eventName, api.Working, gs.kind.workingText()))
	}
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
	if op.Type == OpWaitCondition {
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
