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
	OpRemoveNetwork
	OpDisconnectNetwork
	OpConnectNetwork
	OpCreateVolume
	OpRemoveVolume
	OpCreateContainer
	OpStartContainer
	OpStopContainer
	OpRemoveContainer
	OpRenameContainer
	OpRunPlugin
	OpEmitEvent
)

// String returns a human-readable name for the operation type.
func (o OperationType) String() string {
	switch o {
	case OpCreateNetwork:
		return "create-network"
	case OpRemoveNetwork:
		return "remove-network"
	case OpDisconnectNetwork:
		return "disconnect-network"
	case OpConnectNetwork:
		return "connect-network"
	case OpCreateVolume:
		return "create-volume"
	case OpRemoveVolume:
		return "remove-volume"
	case OpCreateContainer:
		return "create-container"
	case OpStartContainer:
		return "start-container"
	case OpStopContainer:
		return "stop-container"
	case OpRemoveContainer:
		return "remove-container"
	case OpRenameContainer:
		return "rename-container"
	case OpRunPlugin:
		return "run-plugin"
	case OpEmitEvent:
		return "emit-event"
	default:
		return fmt.Sprintf("unknown(%d)", int(o))
	}
}

// Operation describes a single unit of work produced by the reconciliation algorithm.
type Operation struct {
	ID                 string
	Type               OperationType
	ServiceName        string
	Resource           string
	NetworkOp          *NetworkOperation
	VolumeOp           *VolumeOperation
	ContainerOp        *ContainerOperation
	PluginOp           *PluginOperation
	ContainerNetworkOp *ContainerNetworkOperation
	RenameOp           *RenameOperation
	EventOp            *EventOperation
	DependsOn          []string
	Reason             string
}

// NetworkOperation holds details for network create/recreate/remove operations.
type NetworkOperation struct {
	NetworkKey string
	Desired    *types.NetworkConfig
	Existing   *ObservedNetwork
}

// ContainerNetworkOperation holds details for connecting or disconnecting a container from a network.
type ContainerNetworkOperation struct {
	NetworkName string
	ContainerID string
}

// VolumeOperation holds details for volume create/recreate/remove operations.
type VolumeOperation struct {
	VolumeKey string
	Desired   *types.VolumeConfig
	Existing  *ObservedVolume
}

// ContainerOperation holds details for container create/start/stop/remove operations.
type ContainerOperation struct {
	Service         types.ServiceConfig
	ContainerName   string
	ContainerNumber int
	Existing        *container.Summary
	Inherit         bool
	Timeout         *time.Duration
	NetworkRecreate bool // true when this op was created for a network recreation
}

// RenameOperation holds details for renaming a container.
type RenameOperation struct {
	CurrentName string
	NewName     string
}

// EventOperation holds details for emitting a progress event.
type EventOperation struct {
	EventName string
	Status    api.EventStatus // api.Working, api.Done, api.Error
	Text      string          // e.g. "Recreate", "Recreated", "Creating", "Created"
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

// ContainerTouched reports whether the plan contains any operation that
// directly affects the named container (stop, start, create, or remove).
func (p *ReconciliationPlan) ContainerTouched(containerName string) bool {
	for _, op := range p.Operations {
		if op.ContainerOp != nil && op.ContainerOp.ContainerName == containerName {
			return true
		}
	}
	return false
}

// String returns a deterministic, test-friendly dump of the plan.
//
// Operations are listed in topological order, each prefixed by a sequential
// number. Operations that depend on earlier ones show their dependency
// numbers in brackets before an arrow.
//
// Example output:
//
//  1. create network testproject_default  reason: network does not exist
//  2. create volume testproject_myvol  reason: volume does not exist
//     [1,2] -> 3. create container testproject-web-1  reason: scale up
//     [3] -> 4. start container testproject-web-1  reason: container not running
func (p *ReconciliationPlan) String() string {
	if p.IsEmpty() {
		return "(empty plan)"
	}

	ops, err := topologicalSort(p)
	if err != nil {
		return fmt.Sprintf("(invalid plan: %s)", err)
	}

	// Assign a 1-based index to each operation ID
	index := make(map[string]int, len(ops))
	for i, op := range ops {
		index[op.ID] = i + 1
	}

	var b strings.Builder
	b.WriteByte('\n')
	for _, op := range ops {
		if len(op.DependsOn) > 0 {
			depNums := make([]string, 0, len(op.DependsOn))
			sorted := make([]string, len(op.DependsOn))
			copy(sorted, op.DependsOn)
			sort.Strings(sorted)
			for _, depID := range sorted {
				depNums = append(depNums, strconv.Itoa(index[depID]))
			}
			fmt.Fprintf(&b, "[%s] -> ", strings.Join(depNums, ","))
		}
		fmt.Fprintf(&b, "%d. %s %s %s  reason: %s\n", index[op.ID], opVerb(op.Type), opKind(op), op.Resource, op.Reason)
	}
	return b.String()
}

// opKind returns a resource-type label for the operation (network, volume, container, plugin).
func opKind(op *Operation) string {
	switch {
	case op.NetworkOp != nil:
		return "network"
	case op.ContainerNetworkOp != nil:
		return "network"
	case op.VolumeOp != nil:
		return "volume"
	case op.PluginOp != nil:
		return "plugin"
	case op.EventOp != nil:
		return "event"
	default:
		return "container"
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
	StartContainers      bool // include OpStartContainer in the plan (true for up, false for create)
}

// Reconcile computes the set of operations needed to bring the observed state
// in line with the desired project configuration. It is a pure function: it
// makes no Docker API calls and has no side effects.
func Reconcile(project *types.Project, observed *ObservedState, opts ReconcileOptions) (*ReconciliationPlan, error) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{},
		Dependents: map[string][]string{},
	}

	// Expand targeted services to include all transitive dependencies.
	// Keep the original list for recreate policy decisions.
	targetedServices := opts.Services
	if len(opts.Services) > 0 {
		opts.Services = expandServiceDependencies(project, opts.Services)
	}

	// Step 1 - Networks
	if err := reconcileNetworks(project, observed, plan, opts); err != nil {
		return nil, err
	}

	// Step 2 - Volumes
	if err := reconcileVolumes(project, observed, plan, opts); err != nil {
		return nil, err
	}

	// Collect volume keys being recreated so we can force-recreate containers using them
	recreatedVolumes := map[string]bool{}
	for _, op := range plan.Operations {
		if op.Type == OpRemoveVolume && op.VolumeOp != nil {
			recreatedVolumes[op.VolumeOp.VolumeKey] = true
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

	// Populate networkIDs and volumeNames for external resources.
	// External networks/volumes lack the project label, so they won't appear
	// in observed.Networks/Volumes. We scan containers to find the actual IDs.
	resolveExternalNetworkIDs(project, observed, networkIDs)
	resolveExternalVolumeNames(project, observed, volumeNames)

	// Step 3 - Containers per service
	if err := reconcileServices(project, observed, networkIDs, volumeNames, recreatedVolumes, plan, opts, targetedServices); err != nil {
		return nil, err
	}

	// Step 3b - Clean up stale network disconnect/connect operations.
	// When a container is being recreated (has a rename op in the plan),
	// any disconnect/connect ops created by reconcileNetworks use the old
	// container ID which will be stale after recreation. The new container
	// will be connected to the recreated network automatically via
	// buildDependencyEdges (create-container depends on create-network).
	pruneStaleNetworkOpsForRecreatedContainers(plan)

	// Step 4 - Orphans
	if opts.RemoveOrphans {
		reconcileOrphans(observed.Orphans, plan, opts)
	}

	// Step 5 - Cascading restarts: when a service is recreated, stop+start
	// dependent services that have restart: true in their dependency config.
	addCascadingRestarts(project, observed, plan, opts)

	// Step 6 - Dependencies
	buildDependencyEdges(project, plan)

	return plan, nil
}

// reconcileServices iterates over project services and adds container or plugin
// operations to the plan for each targeted service.
func reconcileServices(
	project *types.Project,
	observed *ObservedState,
	networkIDs, volumeNames map[string]string,
	recreatedVolumes map[string]bool,
	plan *ReconciliationPlan,
	opts ReconcileOptions,
	targetedServices []string,
) error {
	for _, service := range project.Services {
		if len(opts.Services) > 0 && !slices.Contains(opts.Services, service.Name) {
			continue
		}

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

		if err := reconcileServiceContainers(project, service, observed.Containers[service.Name], networkIDs, volumeNames, recreatedVolumes, plan, opts, targetedServices); err != nil {
			return err
		}
	}
	return nil
}

// reconcileOrphans adds stop+remove operations for orphan containers.
func reconcileOrphans(orphans Containers, plan *ReconciliationPlan, opts ReconcileOptions) {
	for _, ctr := range orphans {
		ctrName := getCanonicalContainerName(ctr)
		serviceName := ctr.Labels[api.ServiceLabel]
		eventName := "Container " + ctrName

		emitStoppingID := fmt.Sprintf("emit-stopping:%s", ctrName)
		plan.Operations[emitStoppingID] = emitEventOp(emitStoppingID, serviceName, ctrName, eventName, api.Working, api.StatusStopping, nil)

		stopID := fmt.Sprintf("stop-container:%s", ctrName)
		plan.Operations[stopID] = &Operation{
			ID:          stopID,
			Type:        OpStopContainer,
			ServiceName: serviceName,
			Resource:    ctrName,
			ContainerOp: &ContainerOperation{
				ContainerName: ctrName,
				Existing:      &ctr,
				Timeout:       opts.Timeout,
			},
			DependsOn: []string{emitStoppingID},
			Reason:    "orphan container",
		}

		emitStoppedID := fmt.Sprintf("emit-stopped:%s", ctrName)
		plan.Operations[emitStoppedID] = emitEventOp(emitStoppedID, serviceName, ctrName, eventName, api.Done, api.StatusStopped, []string{stopID})

		emitRemovingID := fmt.Sprintf("emit-removing:%s", ctrName)
		plan.Operations[emitRemovingID] = emitEventOp(emitRemovingID, serviceName, ctrName, eventName, api.Working, api.StatusRemoving, []string{emitStoppedID})

		removeID := fmt.Sprintf("remove-container:%s", ctrName)
		plan.Operations[removeID] = &Operation{
			ID:          removeID,
			Type:        OpRemoveContainer,
			ServiceName: serviceName,
			Resource:    ctrName,
			ContainerOp: &ContainerOperation{
				ContainerName: ctrName,
				Existing:      &ctr,
				Timeout:       opts.Timeout,
			},
			DependsOn: []string{emitRemovingID},
			Reason:    "orphan container",
		}

		emitRemovedID := fmt.Sprintf("emit-removed:%s", ctrName)
		plan.Operations[emitRemovedID] = emitEventOp(emitRemovedID, serviceName, ctrName, eventName, api.Done, api.StatusRemoved, []string{removeID})
	}
}

// reconcileNetworks adds network create/remove/disconnect operations to the plan.
// When a network must be recreated (config hash diverged), it decomposes the
// recreation into discrete operations: stop containers → disconnect → remove → create,
// each with proper dependency edges.
func reconcileNetworks(project *types.Project, observed *ObservedState, plan *ReconciliationPlan, opts ReconcileOptions) error {
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
			return fmt.Errorf("hashing network %q: %w", key, err)
		}
		if existing.ConfigHash == desiredHash {
			continue
		}

		// Network config has diverged — decompose recreation into:
		// 1. Stop containers connected to this network
		// 2. Disconnect containers from the network
		// 3. Remove the old network
		// 4. Create the new network

		// Find all containers connected to this network
		connectedContainers := findContainersOnNetwork(observed, n.Name, existing.ID)

		var disconnectIDs []string
		for _, ctr := range connectedContainers {
			ctrName := getCanonicalContainerName(ctr)
			serviceName := ctr.Labels[api.ServiceLabel]
			eventName := "Container " + ctrName

			// Emit Stopping event + Stop the container (needed before disconnect)
			stopID := fmt.Sprintf("stop-container:%s", ctrName)
			if _, exists := plan.Operations[stopID]; !exists {
				emitStoppingID := fmt.Sprintf("emit-stopping:%s", ctrName)
				plan.Operations[emitStoppingID] = emitEventOp(emitStoppingID, serviceName, ctrName, eventName, api.Working, api.StatusStopping, nil)

				plan.Operations[stopID] = &Operation{
					ID:          stopID,
					Type:        OpStopContainer,
					ServiceName: serviceName,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						ContainerName:   ctrName,
						Existing:        &ctr,
						Timeout:         opts.Timeout,
						NetworkRecreate: true,
					},
					DependsOn: []string{emitStoppingID},
					Reason:    fmt.Sprintf("network %q is being recreated", n.Name),
				}

				emitStoppedID := fmt.Sprintf("emit-stopped:%s", ctrName)
				plan.Operations[emitStoppedID] = emitEventOp(emitStoppedID, serviceName, ctrName, eventName, api.Done, api.StatusStopped, []string{stopID})
			}

			// Disconnect the container from the network
			disconnectID := fmt.Sprintf("disconnect-network:%s/%s", n.Name, ctrName)
			plan.Operations[disconnectID] = &Operation{
				ID:       disconnectID,
				Type:     OpDisconnectNetwork,
				Resource: fmt.Sprintf("%s from %s", ctrName, n.Name),
				ContainerNetworkOp: &ContainerNetworkOperation{
					NetworkName: n.Name,
					ContainerID: ctr.ID,
				},
				DependsOn: []string{stopID},
				Reason:    fmt.Sprintf("network %q is being recreated", n.Name),
			}
			disconnectIDs = append(disconnectIDs, disconnectID)
		}

		// Remove the old network (depends on all disconnects)
		removeID := fmt.Sprintf("remove-network:%s", n.Name)
		plan.Operations[removeID] = &Operation{
			ID:       removeID,
			Type:     OpRemoveNetwork,
			Resource: n.Name,
			NetworkOp: &NetworkOperation{
				NetworkKey: key,
				Existing:   &existing,
			},
			DependsOn: disconnectIDs,
			Reason:    "config hash changed",
		}

		// Create the new network (depends on remove)
		createID := fmt.Sprintf("create-network:%s", n.Name)
		plan.Operations[createID] = &Operation{
			ID:       createID,
			Type:     OpCreateNetwork,
			Resource: n.Name,
			NetworkOp: &NetworkOperation{
				NetworkKey: key,
				Desired:    &n,
			},
			DependsOn: []string{removeID},
			Reason:    "config hash changed",
		}

		// Reconnect and restart containers
		for _, ctr := range connectedContainers {
			ctrName := getCanonicalContainerName(ctr)
			serviceName := ctr.Labels[api.ServiceLabel]
			eventName := "Container " + ctrName

			// Connect the container to the new network
			connectID := fmt.Sprintf("connect-network:%s/%s", n.Name, ctrName)
			plan.Operations[connectID] = &Operation{
				ID:       connectID,
				Type:     OpConnectNetwork,
				Resource: fmt.Sprintf("%s to %s", ctrName, n.Name),
				ContainerNetworkOp: &ContainerNetworkOperation{
					NetworkName: n.Name,
					ContainerID: ctr.ID,
				},
				DependsOn: []string{createID},
				Reason:    fmt.Sprintf("network %q has been recreated", n.Name),
			}

			// Emit Starting event
			emitStartingID := fmt.Sprintf("emit-starting:%s", ctrName)
			if _, exists := plan.Operations[emitStartingID]; !exists {
				plan.Operations[emitStartingID] = emitEventOp(emitStartingID, serviceName, ctrName, eventName, api.Working, api.StatusStarting, []string{connectID})
			} else if !slices.Contains(plan.Operations[emitStartingID].DependsOn, connectID) {
				// Container is connected to multiple recreated networks;
				// add the connect op as a dependency of the existing emit-starting.
				plan.Operations[emitStartingID].DependsOn = append(plan.Operations[emitStartingID].DependsOn, connectID)
			}

			// Start the container (depends on emit-starting)
			startID := fmt.Sprintf("start-container:%s", ctrName)
			if existingOp, exists := plan.Operations[startID]; exists {
				// Container is connected to multiple recreated networks;
				// the start already exists, make sure it depends on emit-starting.
				if !slices.Contains(existingOp.DependsOn, emitStartingID) {
					existingOp.DependsOn = append(existingOp.DependsOn, emitStartingID)
				}
			} else {
				plan.Operations[startID] = &Operation{
					ID:          startID,
					Type:        OpStartContainer,
					ServiceName: serviceName,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						ContainerName: ctrName,
						Existing:      &ctr,
					},
					DependsOn: []string{emitStartingID},
					Reason:    fmt.Sprintf("network %q has been recreated", n.Name),
				}
			}

			// Emit Started event
			emitStartedID := fmt.Sprintf("emit-started:%s", ctrName)
			if _, exists := plan.Operations[emitStartedID]; !exists {
				plan.Operations[emitStartedID] = emitEventOp(emitStartedID, serviceName, ctrName, eventName, api.Done, api.StatusStarted, []string{startID})
			}
		}
	}
	return nil
}

// findContainersOnNetwork returns all containers from the observed state
// that are connected to the given network (by name or ID).
func findContainersOnNetwork(observed *ObservedState, networkName string, networkID string) []container.Summary {
	var result []container.Summary
	allContainers := observed.allContainers()
	for _, ctr := range allContainers {
		if ctr.NetworkSettings == nil {
			continue
		}
		for name, ep := range ctr.NetworkSettings.Networks {
			if ep != nil && ((networkID != "" && ep.NetworkID == networkID) || (networkName != "" && name == networkName)) {
				result = append(result, ctr)
				break
			}
		}
	}
	return result
}

// reconcileVolumes adds volume create/remove operations to the plan.
// When a volume must be recreated (config hash diverged), it decomposes the
// recreation into discrete operations: stop containers → remove containers → remove volume → create volume,
// each with proper dependency edges.
func reconcileVolumes(project *types.Project, observed *ObservedState, plan *ReconciliationPlan, opts ReconcileOptions) error {
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
			return fmt.Errorf("hashing volume %q: %w", key, err)
		}
		if existing.ConfigHash == desiredHash {
			continue
		}

		// Volume config has diverged — decompose recreation into:
		// 1. Stop containers using this volume
		// 2. Remove containers (required before volume can be removed)
		// 3. Remove the old volume
		// 4. Create the new volume

		connectedContainers := findContainersUsingVolume(project, observed, key)

		var removeContainerIDs []string
		for _, ctr := range connectedContainers {
			ctrName := getCanonicalContainerName(ctr)
			serviceName := ctr.Labels[api.ServiceLabel]
			eventName := "Container " + ctrName

			// Stop the container with event wrapping
			stopID := fmt.Sprintf("stop-container:%s", ctrName)
			if _, exists := plan.Operations[stopID]; !exists {
				emitStoppingID := fmt.Sprintf("emit-stopping:%s", ctrName)
				plan.Operations[emitStoppingID] = emitEventOp(emitStoppingID, serviceName, ctrName, eventName, api.Working, api.StatusStopping, nil)

				plan.Operations[stopID] = &Operation{
					ID:          stopID,
					Type:        OpStopContainer,
					ServiceName: serviceName,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						ContainerName: ctrName,
						Existing:      &ctr,
						Timeout:       opts.Timeout,
					},
					DependsOn: []string{emitStoppingID},
					Reason:    fmt.Sprintf("volume %q is being recreated", v.Name),
				}

				emitStoppedID := fmt.Sprintf("emit-stopped:%s", ctrName)
				plan.Operations[emitStoppedID] = emitEventOp(emitStoppedID, serviceName, ctrName, eventName, api.Done, api.StatusStopped, []string{stopID})
			}

			// Remove the container (volumes require container removal, not just stop)
			removeID := fmt.Sprintf("remove-container:%s", ctrName)
			if _, exists := plan.Operations[removeID]; !exists {
				emitRemovingID := fmt.Sprintf("emit-removing:%s", ctrName)
				plan.Operations[emitRemovingID] = emitEventOp(emitRemovingID, serviceName, ctrName, eventName, api.Working, api.StatusRemoving, []string{"emit-stopped:" + ctrName})

				plan.Operations[removeID] = &Operation{
					ID:          removeID,
					Type:        OpRemoveContainer,
					ServiceName: serviceName,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						ContainerName: ctrName,
						Existing:      &ctr,
						Timeout:       opts.Timeout,
					},
					DependsOn: []string{emitRemovingID},
					Reason:    fmt.Sprintf("volume %q is being recreated", v.Name),
				}

				emitRemovedID := fmt.Sprintf("emit-removed:%s", ctrName)
				plan.Operations[emitRemovedID] = emitEventOp(emitRemovedID, serviceName, ctrName, eventName, api.Done, api.StatusRemoved, []string{removeID})
			}
			removeContainerIDs = append(removeContainerIDs, removeID)
		}

		// Remove the old volume (depends on all container removals)
		removeID := fmt.Sprintf("remove-volume:%s", v.Name)
		plan.Operations[removeID] = &Operation{
			ID:       removeID,
			Type:     OpRemoveVolume,
			Resource: v.Name,
			VolumeOp: &VolumeOperation{
				VolumeKey: key,
				Existing:  &existing,
			},
			DependsOn: removeContainerIDs,
			Reason:    "config hash changed",
		}

		// Create the new volume (depends on remove)
		createID := fmt.Sprintf("create-volume:%s", v.Name)
		plan.Operations[createID] = &Operation{
			ID:       createID,
			Type:     OpCreateVolume,
			Resource: v.Name,
			VolumeOp: &VolumeOperation{
				VolumeKey: key,
				Desired:   &v,
			},
			DependsOn: []string{removeID},
			Reason:    "config hash changed",
		}
	}
	return nil
}

// findContainersUsingVolume returns all containers from the observed state
// that mount the given volume key.
func findContainersUsingVolume(project *types.Project, observed *ObservedState, volumeKey string) []container.Summary {
	// Find services that use this volume
	var result []container.Summary
	for _, service := range project.Services {
		usesVolume := false
		for _, vol := range service.Volumes {
			if vol.Type == string(mmount.TypeVolume) && vol.Source == volumeKey {
				usesVolume = true
				break
			}
		}
		if !usesVolume {
			continue
		}
		if ctrs, ok := observed.Containers[service.Name]; ok {
			result = append(result, ctrs...)
		}
	}
	return result
}

// resolveExternalNetworkIDs populates networkIDs with IDs for external networks
// by scanning observed containers' network settings.
func resolveExternalNetworkIDs(project *types.Project, observed *ObservedState, networkIDs map[string]string) {
	for key, net := range project.Networks {
		if !net.External || networkIDs[key] != "" {
			continue
		}
		for _, ctrs := range observed.Containers {
			for _, ctr := range ctrs {
				if ctr.NetworkSettings == nil {
					continue
				}
				for netName, ep := range ctr.NetworkSettings.Networks {
					if ep != nil && netName == net.Name {
						networkIDs[key] = ep.NetworkID
					}
				}
			}
		}
	}
}

// resolveExternalVolumeNames populates volumeNames with names for external volumes
// by scanning observed containers' mounts.
func resolveExternalVolumeNames(project *types.Project, observed *ObservedState, volumeNames map[string]string) {
	for key, vol := range project.Volumes {
		if !vol.External || volumeNames[key] != "" {
			continue
		}
		for _, ctrs := range observed.Containers {
			for _, ctr := range ctrs {
				for _, mount := range ctr.Mounts {
					if mount.Type == mmount.TypeVolume && mount.Name == vol.Name {
						volumeNames[key] = vol.Name
					}
				}
			}
		}
	}
}

// reconcileServiceContainers computes plan operations for a single service's
// containers: scale down, recreate, start stopped, and scale up.
func reconcileServiceContainers(
	project *types.Project,
	service types.ServiceConfig,
	containers []container.Summary,
	networkIDs map[string]string,
	volumeNames map[string]string,
	recreatedVolumes map[string]bool,
	plan *ReconciliationPlan,
	opts ReconcileOptions,
	targetedServices []string,
) error {
	expected, err := getScale(service)
	if err != nil {
		return err
	}

	// Determine recreate policy for this service.
	// Use the original targeted services list (before dependency expansion)
	// so that dependencies get the RecreateDependencies policy, not the main Recreate policy.
	policy := opts.RecreateDependencies
	if len(targetedServices) == 0 || slices.Contains(targetedServices, service.Name) {
		policy = opts.Recreate
	}

	// Precompute which containers are obsolete to avoid repeated hashing in the sort comparator.
	// The error from needsRecreate is intentionally discarded here: this is only used for sorting
	// priority (obsolete containers first). The actual recreate decision — with proper error
	// handling — happens below when building the plan operations.
	obsolete := make(map[string]bool, len(containers))
	for _, ctr := range containers {
		recreate, _, _ := needsRecreate(service, ctr, networkIDs, volumeNames, policy)
		obsolete[ctr.ID] = recreate
	}

	// Sort containers: obsolete first, then by container number ascending after reverse
	sort.Slice(containers, func(i, j int) bool {
		oi, oj := obsolete[containers[i].ID], obsolete[containers[j].ID]
		if oi != oj {
			return oi
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
			// Scale down: emit(Stopping) → stop → emit(Stopped) → emit(Removing) → remove → emit(Removed)
			ctrName := getCanonicalContainerName(ctr)
			eventName := "Container " + ctrName

			emitStoppingID := fmt.Sprintf("emit-stopping:%s", ctrName)
			plan.Operations[emitStoppingID] = emitEventOp(emitStoppingID, service.Name, ctrName, eventName, api.Working, api.StatusStopping, nil)

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
				DependsOn: []string{emitStoppingID},
				Reason:    "scale down",
			}

			emitStoppedID := fmt.Sprintf("emit-stopped:%s", ctrName)
			plan.Operations[emitStoppedID] = emitEventOp(emitStoppedID, service.Name, ctrName, eventName, api.Done, api.StatusStopped, []string{stopID})

			emitRemovingID := fmt.Sprintf("emit-removing:%s", ctrName)
			plan.Operations[emitRemovingID] = emitEventOp(emitRemovingID, service.Name, ctrName, eventName, api.Working, api.StatusRemoving, []string{emitStoppedID})

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
					Timeout:       opts.Timeout,
				},
				DependsOn: []string{emitRemovingID},
				Reason:    "scale down",
			}

			emitRemovedID := fmt.Sprintf("emit-removed:%s", ctrName)
			plan.Operations[emitRemovedID] = emitEventOp(emitRemovedID, service.Name, ctrName, eventName, api.Done, api.StatusRemoved, []string{removeID})

			continue
		}

		// Check if the service uses a volume that is being recreated.
		// In that case, ensureVolume will stop+remove the container internally,
		// so we emit OpCreateContainer since the old
		// container will be gone by the time we execute.
		if volReason := serviceUsesRecreatedVolume(service, recreatedVolumes); volReason != "" {
			ctrName := getCanonicalContainerName(ctr)
			number, _ := strconv.Atoi(ctr.Labels[api.ContainerNumberLabel])
			id := fmt.Sprintf("create-container:%s", ctrName)
			plan.Operations[id] = &Operation{
				ID:          id,
				Type:        OpCreateContainer,
				ServiceName: service.Name,
				Resource:    ctrName,
				ContainerOp: &ContainerOperation{
					Service:         service,
					ContainerName:   ctrName,
					ContainerNumber: number,
					Inherit:         opts.Inherit,
					Timeout:         opts.Timeout,
				},
				Reason: volReason,
			}
			continue
		}

		recreate, reason, err := needsRecreate(service, ctr, networkIDs, volumeNames, policy)
		if err != nil {
			return err
		}
		if recreate {
			ctrName := getCanonicalContainerName(ctr)
			eventName := "Container " + ctrName
			number, _ := strconv.Atoi(ctr.Labels[api.ContainerNumberLabel])
			idPrefix := ctr.ID
			if len(idPrefix) > 12 {
				idPrefix = idPrefix[:12]
			}
			tmpName := fmt.Sprintf("%s_%s", idPrefix, ctrName)

			// 0. Emit "Recreate" event (entry point of the chain)
			emitRecreateID := fmt.Sprintf("emit-recreate:%s", ctrName)
			plan.Operations[emitRecreateID] = emitEventOp(emitRecreateID, service.Name, ctrName, eventName, api.Working, "Recreate", nil)

			// 1. Create new container with temp name (inheriting from old if needed)
			createID := fmt.Sprintf("create-container:%s", tmpName)
			plan.Operations[createID] = &Operation{
				ID:          createID,
				Type:        OpCreateContainer,
				ServiceName: service.Name,
				Resource:    tmpName,
				ContainerOp: &ContainerOperation{
					Service:         service,
					ContainerName:   tmpName,
					ContainerNumber: number,
					Existing:        &ctr,
					Inherit:         opts.Inherit,
				},
				DependsOn: []string{emitRecreateID},
				Reason:    reason,
			}

			// 2. Stop old container (depends on create being ready)
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
				DependsOn: []string{createID},
				Reason:    reason,
			}

			// 3. Remove old container (depends on stop)
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
					Timeout:       opts.Timeout,
				},
				DependsOn: []string{stopID},
				Reason:    reason,
			}

			// 4. Rename new container to final name (depends on remove)
			renameID := fmt.Sprintf("rename-container:%s", ctrName)
			plan.Operations[renameID] = &Operation{
				ID:          renameID,
				Type:        OpRenameContainer,
				ServiceName: service.Name,
				Resource:    ctrName,
				RenameOp: &RenameOperation{
					CurrentName: tmpName,
					NewName:     ctrName,
				},
				DependsOn: []string{removeID},
				Reason:    reason,
			}

			// 5. Start the new container (depends on rename) — only when StartContainers is set.
			// docker compose create should not start containers; docker compose up should.
			lastOpID := renameID
			if opts.StartContainers {
				startID := fmt.Sprintf("start-container:%s", ctrName)
				plan.Operations[startID] = &Operation{
					ID:          startID,
					Type:        OpStartContainer,
					ServiceName: service.Name,
					Resource:    ctrName,
					ContainerOp: &ContainerOperation{
						Service:       service,
						ContainerName: ctrName,
						// Existing is nil here; executePlanStartContainer will
						// look up the container by name after rename.
					},
					DependsOn: []string{renameID},
					Reason:    reason,
				}
				lastOpID = startID
			}

			// 6. Emit "Recreated" event (end of chain)
			emitRecreatedID := fmt.Sprintf("emit-recreated:%s", ctrName)
			plan.Operations[emitRecreatedID] = emitEventOp(emitRecreatedID, service.Name, ctrName, eventName, api.Done, "Recreated", []string{lastOpID})

			continue
		}

		// Container is up-to-date; add a start op if needed
		maybeAddStartOp(service, ctr, plan, opts)
	}

	// Scale up: emit(Creating) → create → emit(Created)
	next := nextContainerNumber(containers)
	for i := 0; i < expected-actual; i++ {
		number := next + i
		name := getContainerName(project.Name, service, number)
		eventName := "Container " + name

		emitCreatingID := fmt.Sprintf("emit-creating:%s", name)
		plan.Operations[emitCreatingID] = emitEventOp(emitCreatingID, service.Name, name, eventName, api.Working, api.StatusCreating, nil)

		createID := fmt.Sprintf("create-container:%s", name)
		plan.Operations[createID] = &Operation{
			ID:          createID,
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
			DependsOn: []string{emitCreatingID},
			Reason:    "scale up",
		}

		emitCreatedID := fmt.Sprintf("emit-created:%s", name)
		plan.Operations[emitCreatedID] = emitEventOp(emitCreatedID, service.Name, name, eventName, api.Done, api.StatusCreated, []string{createID})
	}

	return nil
}

// maybeAddStartOp adds an OpStartContainer for a container that is up-to-date
// but not in a running/created/restarting/exited/removing state, and only when
// StartContainers is enabled.
func maybeAddStartOp(service types.ServiceConfig, ctr container.Summary, plan *ReconciliationPlan, opts ReconcileOptions) {
	switch ctr.State {
	case container.StateRunning, container.StateCreated, container.StateRestarting, container.StateExited, container.StateRemoving:
		return
	}
	if !opts.StartContainers {
		return
	}
	ctrName := getCanonicalContainerName(ctr)
	eventName := "Container " + ctrName
	reason := fmt.Sprintf("container not running (state: %s)", ctr.State)

	emitStartingID := fmt.Sprintf("emit-starting:%s", ctrName)
	plan.Operations[emitStartingID] = emitEventOp(emitStartingID, service.Name, ctrName, eventName, api.Working, api.StatusStarting, nil)

	startID := fmt.Sprintf("start-container:%s", ctrName)
	plan.Operations[startID] = &Operation{
		ID:          startID,
		Type:        OpStartContainer,
		ServiceName: service.Name,
		Resource:    ctrName,
		ContainerOp: &ContainerOperation{
			Service:       service,
			ContainerName: ctrName,
			Existing:      &ctr,
		},
		DependsOn: []string{emitStartingID},
		Reason:    reason,
	}

	emitStartedID := fmt.Sprintf("emit-started:%s", ctrName)
	plan.Operations[emitStartedID] = emitEventOp(emitStartedID, service.Name, ctrName, eventName, api.Done, api.StatusStarted, []string{startID})
}

// buildDependencyEdges wires up DependsOn and Dependents for container operations
// based on service dependencies, network dependencies, and volume dependencies.
//
// Note: dependency edges are added to ALL OpCreateContainer/OpStartContainer ops,
// including recreate temp creates (Existing != nil). This is intentional — the
// recreate chain's internal ordering (create → stop → remove → rename → start)
// is already wired, and network/volume deps on the temp create ensure it waits
// for resource recreation. Excluding temp creates would break recreate+network-recreate
// ordering.
func buildDependencyEdges(project *types.Project, plan *ReconciliationPlan) {
	readyOpsByService := indexReadyOps(plan)
	orphanRemoveIDs := collectOrphanRemoveIDs(plan)

	for _, op := range plan.Operations {
		if (op.Type == OpCreateContainer || op.Type == OpStartContainer) && op.ContainerOp != nil {
			addOperationDependencies(project, plan, op, readyOpsByService)
			// Ensure orphan containers are removed before new containers are
			// created to avoid port conflicts or resource contention.
			addOrphanDependencies(op, orphanRemoveIDs)
		}
	}

	buildReverseEdges(plan)
}

// collectOrphanRemoveIDs returns the IDs of all orphan remove-container operations.
func collectOrphanRemoveIDs(plan *ReconciliationPlan) []string {
	var ids []string
	for _, op := range plan.Operations {
		if op.Type == OpRemoveContainer && op.Reason == "orphan container" {
			ids = append(ids, op.ID)
		}
	}
	return ids
}

// addOrphanDependencies makes an operation depend on all orphan removal ops.
func addOrphanDependencies(op *Operation, orphanRemoveIDs []string) {
	for _, id := range orphanRemoveIDs {
		if !slices.Contains(op.DependsOn, id) {
			op.DependsOn = append(op.DependsOn, id)
		}
	}
}

// indexReadyOps indexes the "service ready" operation for each service name.
// This is the last operation that must complete before dependents can proceed:
//   - Recreate chain: the start after rename (create → stop → remove → rename → start)
//   - Fresh create: the OpCreateContainer itself
//   - Plugin: the OpRunPlugin
//
// For recreate chains, a start op is recognized as the "ready" milestone only when its
// sole dependency is a rename op. This is correct because the recreate chain is built
// deterministically (create → stop → remove → rename → start), and additional deps
// (e.g. network reconnects) are added to the start op only by buildDependencyEdges,
// which runs AFTER indexReadyOps.
func indexReadyOps(plan *ReconciliationPlan) map[string][]string {
	readyOpsByService := map[string][]string{}
	for id, op := range plan.Operations {
		switch op.Type {
		case OpEmitEvent:
			if op.EventOp != nil {
				switch op.EventOp.Text {
				case "Recreated":
					// End of recreate chain — this is the ready op
					readyOpsByService[op.ServiceName] = append(readyOpsByService[op.ServiceName], id)
				case api.StatusCreated:
					// End of fresh create chain — this is the ready op
					readyOpsByService[op.ServiceName] = append(readyOpsByService[op.ServiceName], id)
				}
			}
		case OpCreateContainer:
			// Fresh create without event wrapping (e.g. volume-recreated containers)
			// Only if no emit-created exists for this container.
			if op.ContainerOp != nil && op.ContainerOp.Existing == nil {
				emitCreatedID := fmt.Sprintf("emit-created:%s", op.ContainerOp.ContainerName)
				if _, hasEmit := plan.Operations[emitCreatedID]; !hasEmit {
					readyOpsByService[op.ServiceName] = append(readyOpsByService[op.ServiceName], id)
				}
			}
		case OpRunPlugin:
			readyOpsByService[op.ServiceName] = append(readyOpsByService[op.ServiceName], id)
		}
	}
	return readyOpsByService
}

// buildReverseEdges populates plan.Dependents from each operation's DependsOn list.
func buildReverseEdges(plan *ReconciliationPlan) {
	for _, op := range plan.Operations {
		sort.Strings(op.DependsOn)
		for _, depID := range op.DependsOn {
			plan.Dependents[depID] = append(plan.Dependents[depID], op.ID)
		}
	}
	for depID := range plan.Dependents {
		sort.Strings(plan.Dependents[depID])
	}
}

// addOperationDependencies adds service, network, and volume dependency edges
// to a single container operation.
func addOperationDependencies(project *types.Project, plan *ReconciliationPlan, op *Operation, createOpsByService map[string][]string) {
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
		netOpID := "create-network:" + networkConfig.Name
		if _, exists := plan.Operations[netOpID]; exists {
			if !slices.Contains(op.DependsOn, netOpID) {
				op.DependsOn = append(op.DependsOn, netOpID)
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
		volOpID := "create-volume:" + volConfig.Name
		if _, exists := plan.Operations[volOpID]; exists {
			if !slices.Contains(op.DependsOn, volOpID) {
				op.DependsOn = append(op.DependsOn, volOpID)
			}
		}
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

	if networks != nil && actual.State == container.StateRunning && actual.NetworkSettings != nil {
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

// expandServiceDependencies returns a list that includes all services in
// `services` plus all their transitive dependencies found in the project.
func expandServiceDependencies(project *types.Project, services []string) []string {
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		svc, ok := project.Services[name]
		if !ok {
			return
		}
		for _, dep := range svc.GetDependencies() {
			walk(dep)
		}
	}
	for _, s := range services {
		walk(s)
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// addCascadingRestarts adds stop+start operations for services whose
// dependencies are being recreated and that have restart: true in their
// depends_on config.
func addCascadingRestarts(project *types.Project, observed *ObservedState, plan *ReconciliationPlan, opts ReconcileOptions) {
	// Collect services being recreated.
	recreatedServiceRenameOps := map[string][]string{}
	recreatedServiceEntryOps := map[string][]string{} // emit-recreate IDs (entry points of recreate chains)
	for _, op := range plan.Operations {
		if op.Type == OpRenameContainer {
			recreatedServiceRenameOps[op.ServiceName] = append(recreatedServiceRenameOps[op.ServiceName], op.ID)
		}
		if op.Type == OpEmitEvent && op.EventOp != nil && op.EventOp.Text == "Recreate" {
			recreatedServiceEntryOps[op.ServiceName] = append(recreatedServiceEntryOps[op.ServiceName], op.ID)
		}
	}
	if len(recreatedServiceRenameOps) == 0 {
		return
	}

	// For each service in the project, check if it depends on a recreated service with restart: true
	for _, service := range project.Services {
		for depName, dep := range service.DependsOn {
			if !dep.Restart || len(recreatedServiceRenameOps[depName]) == 0 {
				continue
			}
			for _, ctr := range observed.Containers[service.Name] {
				addCascadingRestartOps(service, ctr, depName, recreatedServiceEntryOps[depName], plan, opts)
			}
			break // Only need to process once per service
		}
	}
}

// addCascadingRestartOps adds stop+start operations for a single container
// of a dependent service whose dependency is being recreated.
func addCascadingRestartOps(
	service types.ServiceConfig,
	ctr container.Summary,
	depName string,
	recreateEntryOps []string,
	plan *ReconciliationPlan,
	opts ReconcileOptions,
) {
	ctrName := getCanonicalContainerName(ctr)
	if _, exists := plan.Operations["stop-container:"+ctrName]; exists {
		return
	}
	if ctr.State != container.StateRunning {
		return
	}

	eventName := "Container " + ctrName

	// Stop chain: emit-stopping → stop → emit-stopped.
	// The stop runs BEFORE the dependency recreate chain starts: the
	// recreate entry point (emit-recreate) is updated to depend on
	// emit-stopped, ensuring the dependent is fully stopped before the
	// dependency is recreated (e.g. fluentd logging driver flushing).
	emitStoppingID := fmt.Sprintf("emit-stopping:%s", ctrName)
	plan.Operations[emitStoppingID] = emitEventOp(emitStoppingID, service.Name, ctrName, eventName, api.Working, api.StatusStopping, nil)

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
		DependsOn: []string{emitStoppingID},
		Reason:    fmt.Sprintf("dependency %q is being recreated (restart: true)", depName),
	}

	emitStoppedID := fmt.Sprintf("emit-stopped:%s", ctrName)
	plan.Operations[emitStoppedID] = emitEventOp(emitStoppedID, service.Name, ctrName, eventName, api.Done, api.StatusStopped, []string{stopID})

	// Block the dependency's recreate chain until the dependent is stopped.
	for _, entryID := range recreateEntryOps {
		if entryOp, ok := plan.Operations[entryID]; ok {
			if !slices.Contains(entryOp.DependsOn, emitStoppedID) {
				entryOp.DependsOn = append(entryOp.DependsOn, emitStoppedID)
			}
		}
	}

	// Start chain: only when StartContainers is set. Otherwise the start
	// is handled by startService via InDependencyOrder, which respects
	// depends_on conditions and gives services time to become ready.
	if !opts.StartContainers {
		return
	}
	emitStartingID := fmt.Sprintf("emit-starting:%s", ctrName)
	startID := fmt.Sprintf("start-container:%s", ctrName)
	if existingStart, exists := plan.Operations[startID]; exists {
		if existingEmitStarting, ok := plan.Operations[emitStartingID]; ok {
			if !slices.Contains(existingEmitStarting.DependsOn, emitStoppedID) {
				existingEmitStarting.DependsOn = append(existingEmitStarting.DependsOn, emitStoppedID)
			}
		} else if !slices.Contains(existingStart.DependsOn, stopID) {
			existingStart.DependsOn = append(existingStart.DependsOn, stopID)
		}
	} else {
		plan.Operations[emitStartingID] = emitEventOp(emitStartingID, service.Name, ctrName, eventName, api.Working, api.StatusStarting, []string{emitStoppedID})
		plan.Operations[startID] = &Operation{
			ID:          startID,
			Type:        OpStartContainer,
			ServiceName: service.Name,
			Resource:    ctrName,
			ContainerOp: &ContainerOperation{
				Service:       service,
				ContainerName: ctrName,
				Existing:      &ctr,
			},
			DependsOn: []string{emitStartingID},
			Reason:    fmt.Sprintf("restart after dependency %q recreated", depName),
		}
		emitStartedID := fmt.Sprintf("emit-started:%s", ctrName)
		plan.Operations[emitStartedID] = emitEventOp(emitStartedID, service.Name, ctrName, eventName, api.Done, api.StatusStarted, []string{startID})
	}
}

// pruneStaleNetworkOpsForRecreatedContainers removes disconnect/connect/start
// operations that were created by reconcileNetworks for containers that are
// also being recreated. When a container is recreated, the old container ID
// used in connect/disconnect ops becomes stale. The new container will
// automatically connect to the new network because buildDependencyEdges
// ensures create-container depends on create-network.
func pruneStaleNetworkOpsForRecreatedContainers(plan *ReconciliationPlan) {
	recreatedContainers := collectRecreatedContainerNames(plan)
	if len(recreatedContainers) == 0 {
		return
	}

	toDelete := findStaleNetworkOps(plan, recreatedContainers)
	deletePlanOps(plan, toDelete)
}

// collectRecreatedContainerNames returns the final names of containers that
// have a recreate chain (identified by rename operations).
func collectRecreatedContainerNames(plan *ReconciliationPlan) map[string]bool {
	result := map[string]bool{}
	for _, op := range plan.Operations {
		if op.Type == OpRenameContainer && op.RenameOp != nil {
			result[op.RenameOp.NewName] = true
		}
	}
	return result
}

// findStaleNetworkOps identifies disconnect/connect/start/stop operations that
// reference containers being recreated. These ops use stale container IDs.
func findStaleNetworkOps(plan *ReconciliationPlan, recreated map[string]bool) []string {
	var toDelete []string
	for id, op := range plan.Operations {
		switch op.Type {
		case OpDisconnectNetwork, OpConnectNetwork:
			if isNetworkOpForRecreatedContainer(op, recreated) {
				toDelete = append(toDelete, id)
			}
		case OpStartContainer:
			if isNetworkStartForRecreatedContainer(plan, op, recreated) {
				toDelete = append(toDelete, id)
			}
		case OpStopContainer:
			if isNetworkStopForRecreatedContainer(op, recreated) {
				toDelete = append(toDelete, id)
			}
		case OpEmitEvent:
			// Remove event ops associated with network recreation for containers
			// that are being recreated (their container IDs will be stale).
			if op.EventOp != nil && recreated[op.Resource] {
				// Check if this event op is part of a network recreation chain
				// by checking if it's associated with a stop/start that would be pruned.
				if isNetworkEventForRecreatedContainer(plan, op, recreated) {
					toDelete = append(toDelete, id)
				}
			}
		}
	}
	return toDelete
}

// isNetworkEventForRecreatedContainer checks whether an event op belongs to a
// network recreation chain for a container that is being recreated.
func isNetworkEventForRecreatedContainer(plan *ReconciliationPlan, op *Operation, recreated map[string]bool) bool {
	if !recreated[op.Resource] {
		return false
	}
	// Check if this event depends on or is depended upon by network recreation ops.
	// An event for a network-recreated container's stop/start chain should be pruned.
	// We check by looking at the dependencies: if this event's deps include a network
	// stop/start, or if a network stop/start depends on this event.
	for _, depID := range op.DependsOn {
		dep, ok := plan.Operations[depID]
		if !ok {
			continue
		}
		if dep.Type == OpStopContainer && isNetworkStopForRecreatedContainer(dep, recreated) {
			return true
		}
		if dep.Type == OpStartContainer && isNetworkStartForRecreatedContainer(plan, dep, recreated) {
			return true
		}
		if dep.Type == OpConnectNetwork && isNetworkOpForRecreatedContainer(dep, recreated) {
			return true
		}
	}
	// Also check if any network stop/start depends on this event
	for _, otherOp := range plan.Operations {
		if !slices.Contains(otherOp.DependsOn, op.ID) {
			continue
		}
		if otherOp.Type == OpStopContainer && isNetworkStopForRecreatedContainer(otherOp, recreated) {
			return true
		}
		if otherOp.Type == OpStartContainer && isNetworkStartForRecreatedContainer(plan, otherOp, recreated) {
			return true
		}
	}
	return false
}

func isNetworkOpForRecreatedContainer(op *Operation, recreated map[string]bool) bool {
	if op.ContainerNetworkOp == nil {
		return false
	}
	parts := strings.Fields(op.Resource)
	return len(parts) > 0 && recreated[parts[0]]
}

func isNetworkStartForRecreatedContainer(plan *ReconciliationPlan, op *Operation, recreated map[string]bool) bool {
	if op.ContainerOp == nil || !recreated[op.ContainerOp.ContainerName] {
		return false
	}
	// Only remove start ops created by network recreation (depend on a connect op),
	// not the start from the recreate chain itself (depends on a rename op).
	for _, depID := range op.DependsOn {
		if dep, ok := plan.Operations[depID]; ok && dep.Type == OpConnectNetwork {
			return true
		}
	}
	return false
}

func isNetworkStopForRecreatedContainer(op *Operation, recreated map[string]bool) bool {
	if op.ContainerOp == nil || !recreated[op.ContainerOp.ContainerName] {
		return false
	}
	// Only remove stop ops from network recreation, not from the recreate chain.
	return op.ContainerOp.NetworkRecreate
}

// deletePlanOps removes operations by ID and cleans up DependsOn references.
func deletePlanOps(plan *ReconciliationPlan, ids []string) {
	deleted := map[string]bool{}
	for _, id := range ids {
		deleted[id] = true
		delete(plan.Operations, id)
	}
	for _, op := range plan.Operations {
		var cleaned []string
		for _, depID := range op.DependsOn {
			if !deleted[depID] {
				cleaned = append(cleaned, depID)
			}
		}
		op.DependsOn = cleaned
	}
}

// emitEventOp creates an Operation of type OpEmitEvent for progress reporting.
func emitEventOp(id, serviceName, resource, eventName string, status api.EventStatus, text string, dependsOn []string) *Operation {
	return &Operation{
		ID:          id,
		Type:        OpEmitEvent,
		ServiceName: serviceName,
		Resource:    resource,
		EventOp: &EventOperation{
			EventName: eventName,
			Status:    status,
			Text:      text,
		},
		DependsOn: dependsOn,
		Reason:    text,
	}
}

// serviceUsesRecreatedVolume checks if a service mounts any volume that is
// being recreated, and returns a reason string if so.
func serviceUsesRecreatedVolume(service types.ServiceConfig, recreatedVolumes map[string]bool) string {
	for _, vol := range service.Volumes {
		if vol.Type != string(mmount.TypeVolume) {
			continue
		}
		if recreatedVolumes[vol.Source] {
			return fmt.Sprintf("volume %q is being recreated", vol.Source)
		}
	}
	return ""
}
