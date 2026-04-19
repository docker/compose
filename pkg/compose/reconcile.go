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

	"github.com/compose-spec/compose-go/v2/types"
)

// ReconcileOptions controls how the reconciler compares desired and observed state.
type ReconcileOptions struct {
	Services             []string // targeted services (empty = all)
	Recreate             string   // "diverged", "force", "never" for targeted services
	RecreateDependencies string   // same for non-targeted services
	Inherit              bool     // inherit anonymous volumes on recreate
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
}

// reconcile is the main entry point: it builds a Plan from desired vs observed state.
// The prompt function is called for interactive decisions (e.g. volume divergence).
func reconcile(_ context.Context, project *types.Project, observed *ObservedState, options ReconcileOptions, prompt Prompt) (*Plan, error) {
	r := &reconciler{
		project:  project,
		observed: observed,
		options:  options,
		prompt:   prompt,
		plan:     &Plan{},
	}

	if err := r.reconcileNetworks(); err != nil {
		return nil, err
	}

	if err := r.reconcileVolumes(); err != nil {
		return nil, err
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

// planCreateNetwork adds a single CreateNetwork node.
func (r *reconciler) planCreateNetwork(key string, nw *types.NetworkConfig) *PlanNode {
	return r.plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      "not found",
		Name:       nw.Name,
		Network:    nw,
	}, "")
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
	r.plan.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: fmt.Sprintf("network:%s", key),
		Cause:      "recreate after config change",
		Name:       nw.Name,
		Network:    nw,
	}, "", removeNode)

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

// planCreateVolume adds a single CreateVolume node.
func (r *reconciler) planCreateVolume(key string, vol *types.VolumeConfig) *PlanNode {
	return r.plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      "not found",
		Name:       vol.Name,
		Volume:     vol,
	}, "")
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
	r.plan.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: fmt.Sprintf("volume:%s", key),
		Cause:      "recreate after config change",
		Name:       vol.Name,
		Volume:     vol,
	}, "", removeVolNode)
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

// serviceLabel is a package-level shorthand for the service label key.
const serviceLabel = "com.docker.compose.service"
