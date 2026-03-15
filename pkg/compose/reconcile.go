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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	mmount "github.com/moby/moby/api/types/mount"

	"github.com/docker/compose/v5/pkg/api"
)

// OperationType represents the kind of reconciliation operation to perform.
type OperationType int

const (
	OpCreateNetwork OperationType = iota
	OpRecreateNetwork
	OpRemoveNetwork
	OpCreateVolume
	OpRecreateVolume
	OpRemoveVolume
	OpCreateContainer
	OpRecreateContainer
	OpStartContainer
	OpStopContainer
	OpRemoveContainer
	OpRunPlugin
)

// String returns a human-readable name for the operation type.
func (o OperationType) String() string {
	switch o {
	case OpCreateNetwork:
		return "create-network"
	case OpRemoveNetwork:
		return "remove-network"
	case OpRecreateNetwork:
		return "recreate-network"
	case OpCreateVolume:
		return "create-volume"
	case OpRemoveVolume:
		return "remove-volume"
	case OpRecreateVolume:
		return "recreate-volume"
	case OpCreateContainer:
		return "create-container"
	case OpRecreateContainer:
		return "recreate-container"
	case OpStartContainer:
		return "start-container"
	case OpStopContainer:
		return "stop-container"
	case OpRemoveContainer:
		return "remove-container"
	case OpRunPlugin:
		return "run-plugin"
	default:
		return fmt.Sprintf("unknown(%d)", int(o))
	}
}

// Operation describes a single unit of work produced by the reconciliation algorithm.
type Operation struct {
	ID          string
	Type        OperationType
	ServiceName string
	Resource    string
	NetworkOp   *NetworkOperation
	VolumeOp    *VolumeOperation
	ContainerOp *ContainerOperation
	PluginOp    *PluginOperation
	DependsOn   []string
	Reason      string
}

// NetworkOperation holds details for network create/recreate/remove operations.
type NetworkOperation struct {
	NetworkKey string
	Desired    *types.NetworkConfig
	Existing   *ObservedNetwork
}

// VolumeOperation holds details for volume create/recreate/remove operations.
type VolumeOperation struct {
	VolumeKey string
	Desired   *types.VolumeConfig
	Existing  *ObservedVolume
}

// ContainerOperation holds details for container create/recreate/start/stop/remove operations.
type ContainerOperation struct {
	Service         types.ServiceConfig
	ContainerName   string
	ContainerNumber int
	Existing        *container.Summary
	Inherit         bool
	Timeout         *time.Duration
}

// PluginOperation holds details for plugin service operations.
type PluginOperation struct {
	Service types.ServiceConfig
	Action  string
}

// ReconciliationPlan holds the full set of operations and their dependency edges.
type ReconciliationPlan struct {
	Operations map[string]*Operation
	Dependents map[string][]string // op ID -> IDs of ops that depend on it
}

// Roots returns all operations that have no dependencies (empty DependsOn).
func (p *ReconciliationPlan) Roots() []*Operation {
	var roots []*Operation
	for _, op := range p.Operations {
		if len(op.DependsOn) == 0 {
			roots = append(roots, op)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].ID < roots[j].ID
	})
	return roots
}

// IsEmpty reports whether the plan contains no operations.
func (p *ReconciliationPlan) IsEmpty() bool {
	return len(p.Operations) == 0
}

// String returns a deterministic, test-friendly dump of the plan.
//
// Each operation is printed on a single line with the format:
//
//	<type>  <resource>  "<reason>"
//
// Operations are grouped by category (Networks, Volumes, Containers, Plugins)
// and sorted alphabetically within each group.
// Dependencies are shown on indented lines below each operation.
//
// Example output:
//
//	Networks:
//	  create   myproject_default  "network does not exist"
//	Containers:
//	  recreate myproject-web-1    "config hash changed"
//	    depends on: create-network:myproject_default
//	  create   myproject-web-2    "scale up"
func (p *ReconciliationPlan) String() string {
	if p.IsEmpty() {
		return "(empty plan)"
	}

	type group struct {
		title string
		ops   []*Operation
	}

	groups := []group{
		{title: "Networks"},
		{title: "Volumes"},
		{title: "Containers"},
		{title: "Plugins"},
	}

	for _, op := range p.Operations {
		switch {
		case op.NetworkOp != nil:
			groups[0].ops = append(groups[0].ops, op)
		case op.VolumeOp != nil:
			groups[1].ops = append(groups[1].ops, op)
		case op.ContainerOp != nil:
			groups[2].ops = append(groups[2].ops, op)
		case op.PluginOp != nil:
			groups[3].ops = append(groups[3].ops, op)
		default:
			groups[2].ops = append(groups[2].ops, op)
		}
	}

	for i := range groups {
		sort.Slice(groups[i].ops, func(a, b int) bool {
			return groups[i].ops[a].ID < groups[i].ops[b].ID
		})
	}

	var b strings.Builder
	for _, g := range groups {
		if len(g.ops) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", g.title)
		for _, op := range g.ops {
			fmt.Fprintf(&b, "  %-10s %s  %q\n", op.Type.verb(), op.Resource, op.Reason)
			if len(op.DependsOn) > 0 {
				deps := make([]string, len(op.DependsOn))
				copy(deps, op.DependsOn)
				sort.Strings(deps)
				fmt.Fprintf(&b, "    depends on: %s\n", strings.Join(deps, ", "))
			}
		}
	}
	return b.String()
}

// verb returns a short action word for the operation type, used in plan display.
func (o OperationType) verb() string {
	switch o {
	case OpCreateNetwork, OpCreateVolume, OpCreateContainer:
		return "create"
	case OpRemoveNetwork, OpRemoveVolume, OpRemoveContainer:
		return "remove"
	case OpRecreateNetwork, OpRecreateVolume, OpRecreateContainer:
		return "recreate"
	case OpStartContainer:
		return "start"
	case OpStopContainer:
		return "stop"
	case OpRunPlugin:
		return "plugin"
	default:
		return "unknown"
	}
}

// ReconcileOptions controls the behavior of the Reconcile function.
type ReconcileOptions struct {
	Recreate             string
	RecreateDependencies string
	Services             []string
	Inherit              bool
	Timeout              *time.Duration
	RemoveOrphans        bool
}

// Reconcile computes the set of operations needed to bring the observed state
// in line with the desired project configuration. It is a pure function: it
// makes no Docker API calls and has no side effects.
func Reconcile(project *types.Project, observed *ObservedState, opts ReconcileOptions) (*ReconciliationPlan, error) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{},
		Dependents: map[string][]string{},
	}

	// Step 1 - Networks
	for key, net := range project.Networks {
		if net.External {
			continue
		}
		n := net
		existing, found := observed.Networks[key]
		if !found {
			id := fmt.Sprintf("create-network:%s", n.Name)
			plan.Operations[id] = &Operation{
				ID:       id,
				Type:     OpCreateNetwork,
				Resource: n.Name,
				NetworkOp: &NetworkOperation{
					NetworkKey: key,
					Desired:    &n,
				},
				Reason: "network does not exist",
			}
			continue
		}
		desiredHash, err := NetworkHash(&n)
		if err != nil {
			return nil, fmt.Errorf("hashing network %q: %w", key, err)
		}
		if existing.ConfigHash != desiredHash {
			id := fmt.Sprintf("recreate-network:%s", n.Name)
			plan.Operations[id] = &Operation{
				ID:       id,
				Type:     OpRecreateNetwork,
				Resource: n.Name,
				NetworkOp: &NetworkOperation{
					NetworkKey: key,
					Desired:    &n,
					Existing:   &existing,
				},
				Reason: "config hash changed",
			}
		}
	}

	// Step 2 - Volumes
	for key, vol := range project.Volumes {
		if vol.External {
			continue
		}
		v := vol
		existing, found := observed.Volumes[key]
		if !found {
			id := fmt.Sprintf("create-volume:%s", v.Name)
			plan.Operations[id] = &Operation{
				ID:       id,
				Type:     OpCreateVolume,
				Resource: v.Name,
				VolumeOp: &VolumeOperation{
					VolumeKey: key,
					Desired:   &v,
				},
				Reason: "volume does not exist",
			}
			continue
		}
		desiredHash, err := VolumeHash(v)
		if err != nil {
			return nil, fmt.Errorf("hashing volume %q: %w", key, err)
		}
		if existing.ConfigHash != desiredHash {
			id := fmt.Sprintf("recreate-volume:%s", v.Name)
			plan.Operations[id] = &Operation{
				ID:       id,
				Type:     OpRecreateVolume,
				Resource: v.Name,
				VolumeOp: &VolumeOperation{
					VolumeKey: key,
					Desired:   &v,
					Existing:  &existing,
				},
				Reason: "config hash changed",
			}
		}
	}

	// Build network ID map and volume name map for needsRecreate checks
	networkIDs := map[string]string{}
	for key, n := range observed.Networks {
		networkIDs[key] = n.ID
	}
	volumeNames := map[string]string{}
	for key, v := range observed.Volumes {
		volumeNames[key] = v.Name
	}

	// Step 3 - Containers per service
	for _, service := range project.Services {
		if len(opts.Services) > 0 && !slices.Contains(opts.Services, service.Name) {
			continue
		}

		// Plugin services
		if service.Provider != nil {
			id := fmt.Sprintf("run-plugin:%s", service.Name)
			plan.Operations[id] = &Operation{
				ID:          id,
				Type:        OpRunPlugin,
				ServiceName: service.Name,
				Resource:    service.Name,
				PluginOp: &PluginOperation{
					Service: service,
					Action:  "up",
				},
				Reason: "plugin service",
			}
			continue
		}

		expected, err := getScale(service)
		if err != nil {
			return nil, err
		}

		containers := observed.Containers[service.Name]

		// Determine recreate policy for this service
		policy := opts.RecreateDependencies
		if len(opts.Services) == 0 || slices.Contains(opts.Services, service.Name) {
			policy = opts.Recreate
		}

		// Sort containers: obsolete first, then by container number ascending after reverse
		sort.Slice(containers, func(i, j int) bool {
			obsoleteI, _, _ := needsRecreate(service, containers[i], networkIDs, volumeNames, policy)
			if obsoleteI {
				return true
			}
			obsoleteJ, _, _ := needsRecreate(service, containers[j], networkIDs, volumeNames, policy)
			if obsoleteJ {
				return false
			}

			ni, erri := strconv.Atoi(containers[i].Labels[api.ContainerNumberLabel])
			nj, errj := strconv.Atoi(containers[j].Labels[api.ContainerNumberLabel])
			if erri == nil && errj == nil {
				return ni > nj
			}
			return containers[i].Created < containers[j].Created
		})
		slices.Reverse(containers)

		actual := len(containers)
		for i, ctr := range containers {
			if i >= expected {
				// Scale down: stop + remove
				ctrName := getCanonicalContainerName(ctr)
				stopID := fmt.Sprintf("stop-container:%s", ctrName)
				plan.Operations[stopID] = &Operation{
					ID:          stopID,
					Type:        OpStopContainer,
					ServiceName: service.Name,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						Service:       service,
						ContainerName: ctrName,
						Existing:      &ctr,
						Timeout:       opts.Timeout,
					},
					Reason: "scale down",
				}
				removeID := fmt.Sprintf("remove-container:%s", ctrName)
				plan.Operations[removeID] = &Operation{
					ID:          removeID,
					Type:        OpRemoveContainer,
					ServiceName: service.Name,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						Service:       service,
						ContainerName: ctrName,
						Existing:      &ctr,
					},
					DependsOn: []string{stopID},
					Reason:    "scale down",
				}
				continue
			}

			recreate, reason, err := needsRecreate(service, ctr, networkIDs, volumeNames, policy)
			if err != nil {
				return nil, err
			}
			if recreate {
				ctrName := getCanonicalContainerName(ctr)
				id := fmt.Sprintf("recreate-container:%s", ctrName)
				plan.Operations[id] = &Operation{
					ID:          id,
					Type:        OpRecreateContainer,
					ServiceName: service.Name,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						Service:       service,
						ContainerName: ctrName,
						Existing:      &ctr,
						Inherit:       opts.Inherit,
						Timeout:       opts.Timeout,
					},
					Reason: reason,
				}
				continue
			}

			// Container is up-to-date; check if it needs starting
			switch ctr.State {
			case container.StateRunning, container.StateCreated, container.StateRestarting, container.StateExited:
				// no action needed
			default:
				ctrName := getCanonicalContainerName(ctr)
				id := fmt.Sprintf("start-container:%s", ctrName)
				plan.Operations[id] = &Operation{
					ID:          id,
					Type:        OpStartContainer,
					ServiceName: service.Name,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						Service:       service,
						ContainerName: ctrName,
						Existing:      &ctr,
					},
					Reason: fmt.Sprintf("container not running (state: %s)", ctr.State),
				}
			}
		}

		// Scale up: create missing containers
		next := nextContainerNumber(containers)
		for i := 0; i < expected-actual; i++ {
			number := next + i
			name := getContainerName(project.Name, service, number)
			id := fmt.Sprintf("create-container:%s", name)
			plan.Operations[id] = &Operation{
				ID:          id,
				Type:        OpCreateContainer,
				ServiceName: service.Name,
				Resource:    name,
				ContainerOp: &ContainerOperation{
					Service:         service,
					ContainerName:   name,
					ContainerNumber: number,
					Inherit:         opts.Inherit,
					Timeout:         opts.Timeout,
				},
				Reason: "scale up",
			}
		}
	}

	// Step 4 - Orphans
	if opts.RemoveOrphans {
		for _, ctr := range observed.Orphans {
			ctrName := getCanonicalContainerName(ctr)
			stopID := fmt.Sprintf("stop-container:%s", ctrName)
			plan.Operations[stopID] = &Operation{
				ID:          stopID,
				Type:        OpStopContainer,
				ServiceName: ctr.Labels[api.ServiceLabel],
				Resource:    ctrName,
				ContainerOp: &ContainerOperation{
					ContainerName: ctrName,
					Existing:      &ctr,
					Timeout:       opts.Timeout,
				},
				Reason: "orphan container",
			}
			removeID := fmt.Sprintf("remove-container:%s", ctrName)
			plan.Operations[removeID] = &Operation{
				ID:          removeID,
				Type:        OpRemoveContainer,
				ServiceName: ctr.Labels[api.ServiceLabel],
				Resource:    ctrName,
				ContainerOp: &ContainerOperation{
					ContainerName: ctrName,
					Existing:      &ctr,
				},
				DependsOn: []string{stopID},
				Reason:    "orphan container",
			}
		}
	}

	// Step 5 - Dependencies
	buildDependencyEdges(project, plan)

	return plan, nil
}

// buildDependencyEdges wires up DependsOn and Dependents for container operations
// based on service dependencies, network dependencies, and volume dependencies.
func buildDependencyEdges(project *types.Project, plan *ReconciliationPlan) {
	// Index create-container ops by service name
	createOpsByService := map[string][]string{}
	for id, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ContainerOp != nil {
			svcName := op.ServiceName
			createOpsByService[svcName] = append(createOpsByService[svcName], id)
		}
	}

	for _, op := range plan.Operations {
		if op.Type != OpCreateContainer && op.Type != OpRecreateContainer && op.Type != OpStartContainer {
			continue
		}
		if op.ContainerOp == nil {
			continue
		}

		service := op.ContainerOp.Service

		// Depend on create ops for dependency services
		for _, depName := range service.GetDependencies() {
			for _, depOpID := range createOpsByService[depName] {
				if !slices.Contains(op.DependsOn, depOpID) {
					op.DependsOn = append(op.DependsOn, depOpID)
				}
			}
		}

		// Depend on network create ops for networks used by this service
		for net := range service.Networks {
			networkConfig, ok := project.Networks[net]
			if !ok {
				continue
			}
			createNetID := fmt.Sprintf("create-network:%s", networkConfig.Name)
			if _, exists := plan.Operations[createNetID]; exists {
				if !slices.Contains(op.DependsOn, createNetID) {
					op.DependsOn = append(op.DependsOn, createNetID)
				}
			}
		}

		// Depend on volume create ops for volumes used by this service
		for _, vol := range service.Volumes {
			if vol.Type != string(mmount.TypeVolume) {
				continue
			}
			if vol.Source == "" {
				continue
			}
			volConfig, ok := project.Volumes[vol.Source]
			if !ok {
				continue
			}
			createVolID := fmt.Sprintf("create-volume:%s", volConfig.Name)
			if _, exists := plan.Operations[createVolID]; exists {
				if !slices.Contains(op.DependsOn, createVolID) {
					op.DependsOn = append(op.DependsOn, createVolID)
				}
			}
		}

		// Sort DependsOn for deterministic output
		sort.Strings(op.DependsOn)

		// Build reverse map (Dependents)
		for _, depID := range op.DependsOn {
			plan.Dependents[depID] = append(plan.Dependents[depID], op.ID)
		}
	}

	// Sort Dependents lists for deterministic output
	for depID := range plan.Dependents {
		sort.Strings(plan.Dependents[depID])
	}
}

// needsRecreate determines whether a container must be recreated based on the
// service configuration, observed state, and recreate policy. It returns whether
// recreation is needed, the reason string, and any error.
func needsRecreate(expected types.ServiceConfig, actual container.Summary, networks map[string]string, volumes map[string]string, policy string) (bool, string, error) {
	if policy == api.RecreateNever {
		return false, "", nil
	}
	if policy == api.RecreateForce {
		return true, "force recreate", nil
	}

	configHash, err := ServiceHash(expected)
	if err != nil {
		return false, "", err
	}
	if actual.Labels[api.ConfigHashLabel] != configHash {
		return true, "config hash changed", nil
	}

	if actual.Labels[api.ImageDigestLabel] != expected.CustomLabels[api.ImageDigestLabel] {
		return true, "image digest changed", nil
	}

	if networks != nil && actual.State == container.StateRunning {
		if checkExpectedNetworks(expected, actual, networks) {
			return true, "network configuration changed", nil
		}
	}

	if volumes != nil {
		if checkExpectedVolumes(expected, actual, volumes) {
			return true, "volume configuration changed", nil
		}
	}

	return false, "", nil
}
