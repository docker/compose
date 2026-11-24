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
	"strings"
	"unicode"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/internal/status"
	"github.com/docker/compose/v5/pkg/api"
)

// statusPublisher is what the reconciliation publishes its plan and its
// progress to: the status of the operation (see status.Tracker).
type statusPublisher interface {
	Plan(steps []api.PlanStep, pending []api.ResourceStatus)
	Step(id int, state api.StepState, message string)
}

// noStatus publishes nowhere, for a service whose event processor keeps no
// status.
type noStatus struct{}

func (noStatus) Plan([]api.PlanStep, []api.ResourceStatus) {}
func (noStatus) Step(int, api.StepState, string)           {}

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

	for _, node := range plan.Nodes {
		op := node.Operation
		id := progressName(op)
		if node.Group != "" {
			id = groups.resourceName(node)
		}
		kind, name, hasRow := status.SplitID(id)
		service := ""
		if op.Service != nil {
			service = op.Service.Name
		}
		step := api.PlanStep{ID: node.ID, Operation: operationName(op.Type), Service: service, Reason: op.Cause}
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
				reasons[id] = userReason(op.Cause)
			}
		}
		steps = append(steps, step)
	}
	for i := range rows {
		id := rows[i].ID()
		rows[i].Message = describePending(actions[id], reasons[id])
	}
	pub.Plan(steps, rows)
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

// internalCauses are the causes of an operation that describe how the plan is
// built rather than why something has to change.
var internalCauses = []string{
	"replaced by", "finalize recreate", "config changed (tmpName)", "start", "pre_start hook",
	"post_start hooks", "pre_start hooks", "no existing container", "depends_on condition",
	"stale pre_start hook container",
}

// userReason returns the cause of an operation when it tells a user why the
// resource changes, and nothing for the ones that are plumbing.
func userReason(cause string) string {
	for _, internal := range internalCauses {
		if strings.HasPrefix(cause, internal) {
			return ""
		}
	}
	return cause
}

// operationName is the name of an operation in the status, in snake case:
// "create_container".
func operationName(t planning.OperationType) string {
	var b strings.Builder
	for i, r := range t.String() {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
