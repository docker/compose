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
