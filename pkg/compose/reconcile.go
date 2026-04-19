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
	"sort"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	mmount "github.com/moby/moby/api/types/mount"

	"github.com/docker/compose/v5/pkg/api"
)

// ReconcileOptions controls how the reconciler compares desired and observed state.
type ReconcileOptions struct {
	Services             []string       // targeted services (empty = all)
	Recreate             string         // "diverged", "force", "never" for targeted services
	RecreateDependencies string         // same for non-targeted services
	Inherit              bool           // inherit anonymous volumes on recreate
	Timeout              *time.Duration // for stop operations
	RemoveOrphans        bool
	SkipProviders        bool
}

// reconciler compares a types.Project (desired state) with an ObservedState
// (actual state) and produces a Plan — a DAG of atomic operations.
type reconciler struct {
	project  *types.Project
	observed *ObservedState
	options  ReconcileOptions
	prompt   Prompt
	plan     *Plan

	// networkNodes and volumeNodes track the last plan node for each
	// network/volume, so container creation nodes can depend on them.
	networkNodes map[string]*PlanNode // compose network key → create node
	volumeNodes  map[string]*PlanNode // compose volume key → create node
	// serviceNodes tracks the last plan node per service, so dependent
	// services can order their operations after dependencies.
	serviceNodes map[string]*PlanNode
}

// reconcile is the main entry point: it builds a Plan from desired vs observed state.
// The prompt function is called for interactive decisions (e.g. volume divergence).
func reconcile(_ context.Context, project *types.Project, observed *ObservedState, options ReconcileOptions, prompt Prompt) (*Plan, error) {
	r := &reconciler{
		project:      project,
		observed:     observed,
		options:      options,
		prompt:       prompt,
		plan:         &Plan{},
		networkNodes: map[string]*PlanNode{},
		volumeNodes:  map[string]*PlanNode{},
		serviceNodes: map[string]*PlanNode{},
	}

	if err := r.reconcileNetworks(); err != nil {
		return nil, err
	}

	if err := r.reconcileVolumes(); err != nil {
		return nil, err
	}

	if err := r.reconcileContainers(); err != nil {
		return nil, err
	}

	if r.options.RemoveOrphans {
		r.reconcileOrphans()
	}

	return r.plan, nil
}

// reconcileNetworks adds plan nodes for network creation or recreation.
func (r *reconciler) reconcileNetworks() error {
	for key, desired := range r.project.Networks {
		if desired.External {
			continue
		}
		observed, exists := r.observed.Networks[key]
		if !exists {
			r.planCreateNetwork(key, &desired)
			continue
		}

		expectedHash, err := NetworkHash(&desired)
		if err != nil {
			return err
		}
		if observed.ConfigHash != "" && observed.ConfigHash != expectedHash {
			if err := r.planRecreateNetwork(key, &desired); err != nil {
				return err
			}
		}
		// else: network exists and config matches, nothing to do
	}
	return nil
}

// planCreateNetwork adds a single CreateNetwork node and records it for dependency tracking.
func (r *reconciler) planCreateNetwork(key string, nw *types.NetworkConfig) *PlanNode {
	node := r.plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      "not found",
		Name:       nw.Name,
		Network:    nw,
	}, "")
	r.networkNodes[key] = node
	return node
}

// planRecreateNetwork adds the full sequence for a diverged network:
// stop affected containers → disconnect → remove network → create network.
func (r *reconciler) planRecreateNetwork(key string, nw *types.NetworkConfig) error {
	affectedServices := r.servicesUsingNetwork(key)
	affectedContainers := r.containersForServices(affectedServices)

	// Stop all affected containers
	var stopNodes []*PlanNode
	for i := range affectedContainers {
		oc := &affectedContainers[i]
		node := r.plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: fmt.Sprintf("service:%s:%d", oc.Summary.Labels[serviceLabel], oc.Number),
			Cause:      fmt.Sprintf("network %s config changed", key),
			Container:  &oc.Summary,
		}, "")
		stopNodes = append(stopNodes, node)
	}

	// Disconnect all affected containers (each depends on its own stop)
	var disconnectNodes []*PlanNode
	for i, oc := range affectedContainers {
		node := r.plan.addNode(Operation{
			Type:       OpDisconnectNetwork,
			ResourceID: fmt.Sprintf("service:%s:%d", oc.Summary.Labels[serviceLabel], oc.Number),
			Cause:      fmt.Sprintf("network %s recreate", key),
			Container:  &affectedContainers[i].Summary,
			Name:       nw.Name,
		}, "", stopNodes[i])
		disconnectNodes = append(disconnectNodes, node)
	}

	// Remove network (depends on all disconnects)
	removeNode := r.plan.addNode(Operation{
		Type:       OpRemoveNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      "config hash diverged",
		Name:       nw.Name,
		Network:    nw,
	}, "", disconnectNodes...)

	// Create network (depends on remove)
	createNode := r.plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      "recreate after config change",
		Name:       nw.Name,
		Network:    nw,
	}, "", removeNode)
	r.networkNodes[key] = createNode

	return nil
}

// reconcileVolumes adds plan nodes for volume creation or recreation.
func (r *reconciler) reconcileVolumes() error {
	for key, desired := range r.project.Volumes {
		if desired.External {
			continue
		}
		observed, exists := r.observed.Volumes[key]
		if !exists {
			r.planCreateVolume(key, &desired)
			continue
		}

		expectedHash, err := VolumeHash(desired)
		if err != nil {
			return err
		}
		if observed.ConfigHash != "" && observed.ConfigHash != expectedHash {
			confirmed, err := r.prompt(
				fmt.Sprintf("Volume %q exists but doesn't match configuration in compose file. Recreate (data will be lost)?", desired.Name),
				false,
			)
			if err != nil {
				return err
			}
			if confirmed {
				r.planRecreateVolume(key, &desired)
			}
		}
		// else: volume exists and config matches, nothing to do
	}
	return nil
}

// planCreateVolume adds a single CreateVolume node and records it for dependency tracking.
func (r *reconciler) planCreateVolume(key string, vol *types.VolumeConfig) *PlanNode {
	node := r.plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      "not found",
		Name:       vol.Name,
		Volume:     vol,
	}, "")
	r.volumeNodes[key] = node
	return node
}

// planRecreateVolume adds the full sequence for a diverged volume:
// stop affected containers → remove containers → remove volume → create volume.
// Containers must be removed (not just stopped) because Docker does not allow
// removing a volume that is referenced by any container, even a stopped one.
func (r *reconciler) planRecreateVolume(key string, vol *types.VolumeConfig) {
	affectedServices := r.servicesUsingVolume(key)
	affectedContainers := r.containersForServices(affectedServices)

	// Stop all affected containers
	var stopNodes []*PlanNode
	for i := range affectedContainers {
		oc := &affectedContainers[i]
		node := r.plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: fmt.Sprintf("service:%s:%d", oc.Summary.Labels[serviceLabel], oc.Number),
			Cause:      fmt.Sprintf("volume %s config changed", key),
			Container:  &oc.Summary,
		}, "")
		stopNodes = append(stopNodes, node)
	}

	// Remove all affected containers (each depends on its own stop)
	var removeNodes []*PlanNode
	for i, oc := range affectedContainers {
		node := r.plan.addNode(Operation{
			Type:       OpRemoveContainer,
			ResourceID: fmt.Sprintf("service:%s:%d", oc.Summary.Labels[serviceLabel], oc.Number),
			Cause:      fmt.Sprintf("volume %s config changed", key),
			Container:  &affectedContainers[i].Summary,
		}, "", stopNodes[i])
		removeNodes = append(removeNodes, node)
	}

	// Remove volume (depends on all container removals)
	removeVolNode := r.plan.addNode(Operation{
		Type:       OpRemoveVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      "config hash diverged",
		Name:       vol.Name,
		Volume:     vol,
	}, "", removeNodes...)

	// Create volume (depends on remove)
	createNode := r.plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      "recreate after config change",
		Name:       vol.Name,
		Volume:     vol,
	}, "", removeVolNode)
	r.volumeNodes[key] = createNode
}

// servicesUsingNetwork returns the names of services that reference the given
// compose network key.
func (r *reconciler) servicesUsingNetwork(networkKey string) []string {
	var names []string
	for _, svc := range r.project.Services {
		if _, ok := svc.Networks[networkKey]; ok {
			names = append(names, svc.Name)
		}
	}
	return names
}

// servicesUsingVolume returns the names of services that mount the given
// compose volume key.
func (r *reconciler) servicesUsingVolume(volumeKey string) []string {
	var names []string
	for _, svc := range r.project.Services {
		for _, v := range svc.Volumes {
			if v.Source == volumeKey {
				names = append(names, svc.Name)
				break
			}
		}
	}
	return names
}

// containersForServices returns all observed containers belonging to the given
// service names.
func (r *reconciler) containersForServices(services []string) []ObservedContainer {
	var result []ObservedContainer
	for _, svc := range services {
		result = append(result, r.observed.Containers[svc]...)
	}
	return result
}

// reconcileContainers processes each service in dependency order, comparing
// the desired scale and configuration with observed containers.
func (r *reconciler) reconcileContainers() error {
	// Build dependency graph and process in order
	graph, err := NewGraph(r.project, ServiceStopped)
	if err != nil {
		return err
	}

	// Visit in dependency order (leaves first = services with no deps)
	return r.visitInDependencyOrder(graph)
}

// visitInDependencyOrder processes services from leaves to roots so that
// dependencies are reconciled before the services that depend on them.
func (r *reconciler) visitInDependencyOrder(g *Graph) error {
	visited := map[string]bool{}
	for {
		// Find a vertex whose all children are visited
		var next *Vertex
		for _, v := range g.Vertices {
			if visited[v.Key] {
				continue
			}
			allChildrenVisited := true
			for _, child := range v.Children {
				if !visited[child.Key] {
					allChildrenVisited = false
					break
				}
			}
			if allChildrenVisited {
				next = v
				break
			}
		}
		if next == nil {
			break // all visited
		}
		visited[next.Key] = true

		service, err := r.project.GetService(next.Service)
		if err != nil {
			return err
		}
		if err := r.reconcileService(service); err != nil {
			return err
		}
	}
	return nil
}

// reconcileService handles a single service: scale down, recreate diverged,
// start stopped, scale up.
func (r *reconciler) reconcileService(service types.ServiceConfig) error {
	if service.Provider != nil && r.options.SkipProviders {
		return nil
	}
	if service.Provider != nil {
		// Provider services are handled by plugins, not by the reconciler
		return nil
	}

	expected, err := getScale(service)
	if err != nil {
		return err
	}

	containers := r.observed.Containers[service.Name]
	actual := len(containers)

	strategy := r.options.RecreateDependencies
	if slices.Contains(r.options.Services, service.Name) || len(r.options.Services) == 0 {
		strategy = r.options.Recreate
	}

	// Sort containers: obsolete first, then by number descending, then reverse
	// to get the same ordering as the existing convergence code.
	r.sortContainers(containers, service, strategy)

	// Collect dependency nodes that container creation should depend on
	infraDeps := r.infrastructureDeps(service)

	var lastNode *PlanNode

	// Process existing containers
	for i, oc := range containers {
		if i >= expected {
			// Scale down: stop + remove excess containers
			stopNode := r.plan.addNode(Operation{
				Type:       OpStopContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "scale down",
				Container:  &containers[i].Summary,
				Timeout:    r.options.Timeout,
			}, "")
			r.plan.addNode(Operation{
				Type:       OpRemoveContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "scale down",
				Container:  &containers[i].Summary,
			}, "", stopNode)
			continue
		}

		recreate, err := r.mustRecreate(service, oc, strategy)
		if err != nil {
			return err
		}
		if recreate {
			lastNode = r.planRecreateContainer(service, &containers[i], infraDeps)
			continue
		}

		// Container is up-to-date
		switch oc.State {
		case container.StateRunning, container.StateCreated, container.StateRestarting, container.StateExited:
			// Nothing to do (exited containers are left as-is, matching convergence.go behavior)
		default:
			// Stopped/exited container that needs starting
			lastNode = r.plan.addNode(Operation{
				Type:       OpStartContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", service.Name, oc.Number),
				Cause:      "not running",
				Container:  &containers[i].Summary,
			}, "", infraDeps...)
		}
	}

	// Scale up: create new containers
	nextNum := nextContainerNumber(r.observedSummaries(service.Name))
	for i := 0; i < expected-actual; i++ {
		number := nextNum + i
		name := getContainerName(r.project.Name, service, number)
		svc := service // copy for pointer stability
		lastNode = r.plan.addNode(Operation{
			Type:       OpCreateContainer,
			ResourceID: fmt.Sprintf("service:%s:%d", service.Name, number),
			Cause:      "no existing container",
			Service:    &svc,
			Number:     number,
			Name:       name,
		}, "", infraDeps...)
	}

	r.serviceNodes[service.Name] = lastNode
	return nil
}

// mustRecreate mirrors the existing convergence.mustRecreate logic.
func (r *reconciler) mustRecreate(expected types.ServiceConfig, oc ObservedContainer, policy string) (bool, error) {
	if policy == api.RecreateNever {
		return false, nil
	}
	if policy == api.RecreateForce {
		return true, nil
	}
	configHash, err := ServiceHash(expected)
	if err != nil {
		return false, err
	}
	if oc.ConfigHash != configHash {
		return true, nil
	}
	if oc.ImageDigest != expected.CustomLabels[api.ImageDigestLabel] {
		return true, nil
	}

	if oc.State == container.StateRunning && r.hasNetworkMismatch(expected, oc) {
		return true, nil
	}
	if r.hasVolumeMismatch(expected, oc) {
		return true, nil
	}

	return false, nil
}

// hasNetworkMismatch checks if the container is not connected to all expected networks.
func (r *reconciler) hasNetworkMismatch(expected types.ServiceConfig, oc ObservedContainer) bool {
	for net := range expected.Networks {
		expectedID := ""
		if obs, ok := r.observed.Networks[net]; ok {
			expectedID = obs.ID
		}
		if expectedID == "" || expectedID == "swarm" {
			continue
		}
		found := false
		for _, netID := range oc.ConnectedNetworks {
			if netID == expectedID {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// hasVolumeMismatch checks if the container is missing any expected volume mounts.
func (r *reconciler) hasVolumeMismatch(expected types.ServiceConfig, oc ObservedContainer) bool {
	for _, vol := range expected.Volumes {
		if vol.Type != string(mmount.TypeVolume) || vol.Source == "" {
			continue
		}
		expectedName := ""
		if obs, ok := r.observed.Volumes[vol.Source]; ok {
			expectedName = obs.Name
		}
		if expectedName == "" {
			continue
		}
		found := false
		for _, m := range oc.Summary.Mounts {
			if m.Type == mmount.TypeVolume && m.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}

// planRecreateContainer decomposes container recreation into 4 atomic operations:
// CreateContainer(tmpName) → StopContainer → RemoveContainer → RenameContainer
func (r *reconciler) planRecreateContainer(service types.ServiceConfig, oc *ObservedContainer, infraDeps []*PlanNode) *PlanNode {
	resID := fmt.Sprintf("service:%s:%d", service.Name, oc.Number)
	group := fmt.Sprintf("recreate:%s:%d", service.Name, oc.Number)
	tmpName := fmt.Sprintf("%s_%s", oc.ID[:min(12, len(oc.ID))], getContainerName(r.project.Name, service, oc.Number))
	svc := service // copy for pointer stability

	// Stop dependents first
	depStopNodes := r.planStopDependents(service)

	// All deps: infrastructure + dependent stops
	allDeps := append(slices.Clone(infraDeps), depStopNodes...)

	var inherited *container.Summary
	if r.options.Inherit {
		inherited = &oc.Summary
	}

	// 1. Create new container with temporary name
	createNode := r.plan.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: resID,
		Cause:      "config changed (tmpName)",
		Service:    &svc,
		Inherited:  inherited,
		Number:     oc.Number,
		Name:       tmpName,
	}, group, allDeps...)

	// 2. Stop old container
	stopNode := r.plan.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: resID,
		Cause:      fmt.Sprintf("replaced by #%d", createNode.ID),
		Container:  &oc.Summary,
		Timeout:    r.options.Timeout,
	}, group, createNode)

	// 3. Remove old container
	removeNode := r.plan.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: resID,
		Cause:      fmt.Sprintf("replaced by #%d", createNode.ID),
		Container:  &oc.Summary,
	}, group, stopNode)

	// 4. Rename to final name
	finalName := getContainerName(r.project.Name, service, oc.Number)
	renameNode := r.plan.addNode(Operation{
		Type:       OpRenameContainer,
		ResourceID: resID,
		Cause:      "finalize recreate",
		Name:       finalName,
	}, group, removeNode)

	return renameNode
}

// planStopDependents plans stop operations for containers of services that
// depend on the given service with restart: true.
func (r *reconciler) planStopDependents(service types.ServiceConfig) []*PlanNode {
	dependents := r.project.GetDependentsForService(service, func(dep types.ServiceDependency) bool {
		return dep.Restart
	})
	var nodes []*PlanNode
	for _, depName := range dependents {
		for i, oc := range r.observed.Containers[depName] {
			node := r.plan.addNode(Operation{
				Type:       OpStopContainer,
				ResourceID: fmt.Sprintf("service:%s:%d", depName, oc.Number),
				Cause:      fmt.Sprintf("dependency %s being recreated", service.Name),
				Container:  &r.observed.Containers[depName][i].Summary,
				Timeout:    r.options.Timeout,
			}, "")
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// infrastructureDeps returns the plan nodes that a container creation for this
// service should depend on: network creates and volume creates that the service
// references, plus the last node of dependency services.
func (r *reconciler) infrastructureDeps(service types.ServiceConfig) []*PlanNode {
	var deps []*PlanNode
	for net := range service.Networks {
		if node, ok := r.networkNodes[net]; ok {
			deps = append(deps, node)
		}
	}
	for _, vol := range service.Volumes {
		if vol.Type == string(mmount.TypeVolume) && vol.Source != "" {
			if node, ok := r.volumeNodes[vol.Source]; ok {
				deps = append(deps, node)
			}
		}
	}
	for depName := range service.DependsOn {
		if node, ok := r.serviceNodes[depName]; ok {
			deps = append(deps, node)
		}
	}
	return deps
}

// sortContainers sorts containers the same way as convergence.go:138-160:
// obsolete first, then by container number descending, then reversed.
func (r *reconciler) sortContainers(containers []ObservedContainer, service types.ServiceConfig, policy string) {
	sort.Slice(containers, func(i, j int) bool {
		obsi, _ := r.mustRecreate(service, containers[i], policy)
		obsj, _ := r.mustRecreate(service, containers[j], policy)
		if obsi != obsj {
			return obsi // obsolete first
		}
		// preserve low container numbers
		if containers[i].Number != containers[j].Number {
			return containers[i].Number > containers[j].Number
		}
		return containers[i].Summary.Created < containers[j].Summary.Created
	})
	slices.Reverse(containers)
}

// reconcileOrphans plans stop + remove for orphaned containers.
func (r *reconciler) reconcileOrphans() {
	for i, oc := range r.observed.Orphans {
		stopNode := r.plan.addNode(Operation{
			Type:       OpStopContainer,
			ResourceID: fmt.Sprintf("orphan:%s", oc.Name),
			Cause:      "orphaned container",
			Container:  &r.observed.Orphans[i].Summary,
			Timeout:    r.options.Timeout,
		}, "")
		r.plan.addNode(Operation{
			Type:       OpRemoveContainer,
			ResourceID: fmt.Sprintf("orphan:%s", oc.Name),
			Cause:      "orphaned container",
			Container:  &r.observed.Orphans[i].Summary,
		}, "", stopNode)
	}
}

// observedSummaries returns the raw container.Summary list for a service,
// needed by nextContainerNumber which expects []container.Summary.
func (r *reconciler) observedSummaries(serviceName string) []container.Summary {
	ocs := r.observed.Containers[serviceName]
	result := make([]container.Summary, len(ocs))
	for i, oc := range ocs {
		result[i] = oc.Summary
	}
	return result
}

// serviceLabel is a package-level shorthand for the service label key.
const serviceLabel = "com.docker.compose.service"
