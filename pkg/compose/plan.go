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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
)

// OperationType identifies the kind of atomic operation in a reconciliation plan.
// Each operation maps to exactly one Docker API call.
type OperationType int

const (
	// Network operations
	OpCreateNetwork OperationType = iota
	OpRemoveNetwork
	OpDisconnectNetwork
	OpConnectNetwork

	// Volume operations
	OpCreateVolume
	OpRemoveVolume

	// Container operations
	OpCreateContainer
	OpStartContainer
	OpStopContainer
	OpRemoveContainer
	OpRenameContainer
)

// String returns the human-readable name of an OperationType.
func (o OperationType) String() string {
	switch o {
	case OpCreateNetwork:
		return "CreateNetwork"
	case OpRemoveNetwork:
		return "RemoveNetwork"
	case OpDisconnectNetwork:
		return "DisconnectNetwork"
	case OpConnectNetwork:
		return "ConnectNetwork"
	case OpCreateVolume:
		return "CreateVolume"
	case OpRemoveVolume:
		return "RemoveVolume"
	case OpCreateContainer:
		return "CreateContainer"
	case OpStartContainer:
		return "StartContainer"
	case OpStopContainer:
		return "StopContainer"
	case OpRemoveContainer:
		return "RemoveContainer"
	case OpRenameContainer:
		return "RenameContainer"
	default:
		return fmt.Sprintf("Unknown(%d)", int(o))
	}
}

// Operation describes a single atomic action to be performed by the executor.
// It carries all the data needed to execute the operation without further
// decision-making.
type Operation struct {
	Type       OperationType
	ResourceID string // e.g. "service:web:1", "network:backend", "volume:data"
	Cause      string // why this operation is needed

	// Resource-specific data (only the relevant fields are set per operation type)
	Service   *types.ServiceConfig // for container operations
	Container *container.Summary   // existing container (for stop/remove)
	Inherited *container.Summary   // container to inherit anonymous volumes from (for create-as-replacement)
	Number    int                  // container replica number (for create)
	Name      string               // target container/resource name
	Network   *types.NetworkConfig // for network operations
	Volume    *types.VolumeConfig  // for volume operations
	Timeout   *time.Duration       // for stop operations
}

// PlanNode is a single node in the reconciliation DAG. It represents one
// atomic operation and its dependencies on other nodes.
type PlanNode struct {
	ID        int // numeric identifier (#1, #2, ...)
	Operation Operation
	DependsOn []*PlanNode // prerequisite operations
	Group     string      // event grouping key (e.g. "recreate:web:1"); empty for ungrouped nodes
}

// Plan is a directed acyclic graph of operations produced by the reconciler.
// Nodes are stored in topological order (dependencies before dependents).
type Plan struct {
	Nodes  []*PlanNode
	nextID int
}

// addNode appends a new node to the plan and returns it.
func (p *Plan) addNode(op Operation, group string, deps ...*PlanNode) *PlanNode {
	p.nextID++
	node := &PlanNode{
		ID:        p.nextID,
		Operation: op,
		DependsOn: deps,
		Group:     group,
	}
	p.Nodes = append(p.Nodes, node)
	return node
}

// String renders the plan as a human-readable graph for testing and debugging.
//
// Format per line: [dep1,dep2] -> #id resource, operation, cause [group]
//
// Examples:
//
//	[] -> #1 network:default, CreateNetwork, not found
//	[1] -> #2 service:web:1, CreateContainer, no existing container
//	[2] -> #3 service:web:1, StopContainer, replaced by #2 [recreate:web:1]
func (p *Plan) String() string {
	var sb strings.Builder
	for _, node := range p.Nodes {
		deps := make([]string, len(node.DependsOn))
		for i, d := range node.DependsOn {
			deps[i] = strconv.Itoa(d.ID)
		}
		fmt.Fprintf(&sb, "[%s] -> #%d %s, %s, %s",
			strings.Join(deps, ","),
			node.ID,
			node.Operation.ResourceID,
			node.Operation.Type,
			node.Operation.Cause,
		)
		if node.Group != "" {
			fmt.Fprintf(&sb, " [%s]", node.Group)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// IsEmpty returns true if the plan contains no operations.
func (p *Plan) IsEmpty() bool {
	return len(p.Nodes) == 0
}
