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
	"slices"
	"strings"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/internal/status"
	"github.com/docker/compose/v5/pkg/api"
)

// statusPublisher is what the reconciliation publishes its plan and its
// progress to: the status of the operation (see status.Tracker).
type statusPublisher interface {
	Plan(steps []api.PlanStep, pending []api.ResourceStatus)
	Step(id int, state api.StepState, message string)
	// Defer tells that operations of the plan run after the current operation
	// ends: they are not settled with it.
	Defer(ids ...int)
}

// noStatus publishes nowhere, for a service whose event processor keeps no
// status.
type noStatus struct{}

func (noStatus) Plan([]api.PlanStep, []api.ResourceStatus) {}
func (noStatus) Step(int, api.StepState, string)           {}
func (noStatus) Defer(...int)                              {}

// statusPublisher returns where the service publishes the status of what it
// reconciles.
func (s *composeService) statusPublisher() statusPublisher {
	if p, ok := s.events.(statusPublisher); ok {
		return p
	}
	return noStatus{}
}

// publishPlan tells the status what the plan is going to do before it does it:
// its operations, and the resources it is going to change, pending until the
// plan reaches them.
func publishPlan(pub statusPublisher, plan *planning.Plan, groups *groupTracker) {
	steps := make([]api.PlanStep, 0, len(plan.Nodes))
	var rows []api.ResourceStatus
	byID := map[string]int{}
	actions := map[string][]string{}
	reasons := map[string]string{}

	nodes := make(map[int]*planning.PlanNode, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes[node.ID] = node
	}

	for _, node := range plan.Nodes {
		op := node.Operation
		id := planRowID(node, groups)
		kind, name, hasRow := status.SplitID(id)
		if created := nodes[op.CreateNodeID]; !hasRow && created != nil {
			// a start-phase operation on a replica the plan itself creates acts
			// on the row of its creation: the one of the container it replaces
			// for a recreate, whose replacement only has a temporary name
			id = planRowID(created, groups)
			kind, name, hasRow = status.SplitID(id)
		}
		service := ""
		switch {
		case op.Service != nil:
			service = op.Service.Name
		case op.Container != nil:
			// stopping or removing a container acts on one that is observed
			service = op.Container.Labels[api.ServiceLabel]
		}
		step := api.PlanStep{ID: node.ID, Operation: operationName(op.Type), Service: service, Reason: op.Reason}
		for _, dep := range node.DependsOn {
			step.DependsOn = append(step.DependsOn, dep.ID)
		}
		if hasRow {
			step.Resource = id
			if _, seen := byID[id]; !seen {
				byID[id] = len(rows)
				rows = append(rows, api.ResourceStatus{Kind: kind, Name: name, Service: service})
			} else if rows[byID[id]].Service == "" {
				rows[byID[id]].Service = service
			}
			actions[id] = appendAction(actions[id], plannedAction(node, groups))
			if reasons[id] == "" {
				reasons[id] = op.Reason
			}
		}
		steps = append(steps, step)
	}
	for i := range rows {
		id := rows[i].ID()
		acts := normalizeActions(actions[id])
		rows[i].Action = strings.Join(acts, ", ")
		rows[i].Message = describePending(acts, reasons[id])
	}
	pub.Plan(steps, rows)
}

// planRowID is the ID of the row a node of the plan changes: the one of its
// group when it has one, else the one of its resource.
func planRowID(node *planning.PlanNode, groups *groupTracker) string {
	if node.Group != "" {
		return groups.resourceName(node)
	}
	return progressName(node.Operation)
}

// plannedAction names what an operation does to its resource, in the words a
// user reads: a replica that is replaced is "recreate", whatever the number of
// operations that takes.
func plannedAction(node *planning.PlanNode, groups *groupTracker) string {
	if node.Group != "" && groups.kindOf(node) == groupRecreate {
		return "recreate"
	}
	switch node.Operation.Type {
	case planning.OpCreateNetwork, planning.OpCreateVolume, planning.OpCreateContainer, planning.OpCreateHookContainer:
		return "create"
	case planning.OpStartContainer, planning.OpRunPreStart, planning.OpRunPostStart:
		return "start"
	case planning.OpStopContainer:
		return "stop"
	case planning.OpRemoveNetwork, planning.OpRemoveVolume, planning.OpRemoveContainer:
		return "remove"
	case planning.OpConnectNetwork, planning.OpDisconnectNetwork:
		return "reconnect"
	case planning.OpRenameContainer:
		return "recreate"
	default:
		return ""
	}
}

func appendAction(actions []string, action string) []string {
	if action == "" {
		return actions
	}
	for _, a := range actions {
		if a == action {
			return actions
		}
	}
	return append(actions, action)
}

// normalizeActions drops what a recreate implies: a replaced container is
// created and started again, which "recreate" already says.
func normalizeActions(actions []string) []string {
	if slices.Contains(actions, "recreate") {
		return []string{"recreate"}
	}
	return actions
}

// describePending says what a pending resource is waiting for.
func describePending(actions []string, reason string) string {
	text := strings.Join(actions, ", ")
	switch {
	case text == "":
		return reason
	case reason == "":
		return text
	default:
		return text + ": " + reason
	}
}

// stepOperations names the operations of a plan in the status. The names are
// part of what a consumer of the status relies on: they are listed here, and
// not derived from the debug form of the type, so that renaming one cannot
// change them.
var stepOperations = map[planning.OperationType]string{
	planning.OpCreateNetwork:     "create_network",
	planning.OpRemoveNetwork:     "remove_network",
	planning.OpDisconnectNetwork: "disconnect_network",
	planning.OpConnectNetwork:    "connect_network",
	planning.OpCreateVolume:      "create_volume",
	planning.OpRemoveVolume:      "remove_volume",
	planning.OpCreateContainer:   "create_container",
	planning.OpStartContainer:    "start_container",
	planning.OpStopContainer:     "stop_container",
	planning.OpRemoveContainer:   "remove_container",
	planning.OpRenameContainer:   "rename_container",
	planning.OpRunProvider:       "run_provider",
	planning.OpWaitCondition:     "wait_condition",
	planning.OpRunPreStart:       "run_pre_start",
	planning.OpRunPostStart:      "run_post_start",

	planning.OpCreateHookContainer: "create_hook_container",
}

// operationName is the name of an operation in the status: "create_container".
func operationName(t planning.OperationType) string {
	if name, ok := stepOperations[t]; ok {
		return name
	}
	return "unknown"
}
