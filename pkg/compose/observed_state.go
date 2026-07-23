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
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
)

// ObservedState captures the current state of all Docker resources belonging to
// a Compose project. It is a snapshot taken before reconciliation so that the
// reconciler can compare desired state (types.Project) with reality without
// making any further API calls.
type ObservedState struct {
	ProjectName string
	Containers  map[string][]ObservedContainer // service name → containers
	Orphans     []ObservedContainer            // containers with no matching service
	Networks    map[string]ObservedNetwork     // compose network key → observed
	Volumes     map[string]ObservedVolume      // compose volume key → observed
}

// ObservedContainer holds the relevant state extracted from a running or stopped
// container, with label values pre-parsed for efficient comparison.
type ObservedContainer struct {
	ID          string
	Name        string
	State       container.ContainerState // "running", "exited", "created", "restarting", etc.
	ConfigHash  string                   // label com.docker.compose.config-hash
	ImageDigest string                   // label com.docker.compose.image
	Number      int                      // label com.docker.compose.container-number

	// ConnectedNetworks maps network IDs found in the container's network
	// settings. Key is the network name as seen by Docker, value is the
	// network ID.
	ConnectedNetworks map[string]string

	// Raw summary kept for the executor which needs it to call Moby APIs.
	Summary container.Summary
}

// ObservedNetwork holds the state of a Docker network that belongs to the
// project, identified by the com.docker.compose.network label.
type ObservedNetwork struct {
	ID          string
	Name        string
	ConfigHash  string // label com.docker.compose.config-hash
	ProjectName string // label com.docker.compose.project
}

// ObservedVolume holds the state of a Docker volume that belongs to the
// project, identified by the com.docker.compose.volume label.
type ObservedVolume struct {
	Name        string
	ConfigHash  string // label com.docker.compose.config-hash
	ProjectName string // label com.docker.compose.project
	Driver      string
}

// collectObservedState queries the Docker daemon for all resources belonging to
// the given project and returns a structured snapshot.
// The project model is used to classify containers by service and to identify
// orphans, and to scope network/volume queries to declared resources.
func (s *composeService) collectObservedState(ctx context.Context, project *types.Project) (*ObservedState, error) {
	state := &ObservedState{
		ProjectName: project.Name,
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	// --- Containers ---
	// Use oneOffInclude to detect orphaned one-off containers (matching the
	// previous behavior of create() which used oneOffInclude + isOrphaned).
	raw, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return nil, err
	}

	knownServices := map[string]bool{}
	for _, svc := range project.Services {
		knownServices[svc.Name] = true
		state.Containers[svc.Name] = nil // ensure key exists even if empty
	}
	for _, ds := range project.DisabledServices {
		knownServices[ds.Name] = true
	}

	for _, c := range raw {
		svcName := c.Labels[api.ServiceLabel]
		if isNotOneOff(c) && knownServices[svcName] {
			state.Containers[svcName] = append(state.Containers[svcName], toObservedContainer(c))
		} else if isOrphaned(project)(c) {
			state.Orphans = append(state.Orphans, toObservedContainer(c))
		}
	}

	// --- Networks ---
	nwList, err := s.apiClient().NetworkList(ctx, client.NetworkListOptions{
		Filters: projectFilter(project.Name),
	})
	if err != nil {
		return nil, err
	}
	for _, nw := range nwList.Items {
		key := nw.Labels[api.NetworkLabel]
		if key == "" {
			continue
		}
		state.Networks[key] = ObservedNetwork{
			ID:          nw.ID,
			Name:        nw.Name,
			ConfigHash:  nw.Labels[api.ConfigHashLabel],
			ProjectName: nw.Labels[api.ProjectLabel],
		}
	}

	// --- Volumes ---
	volList, err := s.apiClient().VolumeList(ctx, client.VolumeListOptions{
		Filters: projectFilter(project.Name),
	})
	if err != nil {
		return nil, err
	}
	for _, vol := range volList.Items {
		key := vol.Labels[api.VolumeLabel]
		if key == "" {
			continue
		}
		state.Volumes[key] = ObservedVolume{
			Name:        vol.Name,
			ConfigHash:  vol.Labels[api.ConfigHashLabel],
			ProjectName: vol.Labels[api.ProjectLabel],
			Driver:      vol.Driver,
		}
	}

	if err := s.discoverUnmanagedNetworks(ctx, project, state); err != nil {
		return nil, err
	}

	if err := s.discoverUnmanagedVolumes(ctx, project, state); err != nil {
		return nil, err
	}

	return state, nil
}

// discoverUnmanagedNetworks augments the observed state with networks that match
// a declared network by name but carry no compose label — pre-label Compose or
// manually created networks, missed by the label-filtered NetworkList. Each is
// recorded as an unmanaged match with an empty ConfigHash: the reconciler then
// reuses it untouched instead of scheduling a CreateNetwork. See
// warnUnmanagedNetworks for the accompanying user warning.
func (s *composeService) discoverUnmanagedNetworks(ctx context.Context, project *types.Project, state *ObservedState) error {
	for _, key := range project.NetworkNames() {
		nw := project.Networks[key]
		if nw.External {
			continue
		}
		if _, ok := state.Networks[key]; ok {
			continue
		}
		inspected, err := s.apiClient().NetworkInspect(ctx, nw.Name, client.NetworkInspectOptions{})
		if err != nil {
			if errdefs.IsNotFound(err) {
				continue // absent: it will be created by the reconciliation plan
			}
			return err
		}
		// NetworkInspect matches on ID prefix, so guard against a partial match
		// (e.g. a network whose ID starts with the requested name).
		if inspected.Network.Name != nw.Name && inspected.Network.ID != nw.Name {
			continue
		}
		state.Networks[key] = ObservedNetwork{
			ID:          inspected.Network.ID,
			Name:        inspected.Network.Name,
			ProjectName: inspected.Network.Labels[api.ProjectLabel],
			// ConfigHash intentionally left empty: the network is not owned by
			// this project, so we must not treat it as diverged and recreate it.
		}
	}
	return nil
}

// discoverUnmanagedVolumes augments the observed state with volumes that match a
// declared volume by name but carry no compose label — pre-label Compose or
// manually created volumes, missed by the label-filtered VolumeList. Each is
// recorded as an unmanaged match with an empty ConfigHash: the reconciler then
// reuses it untouched instead of scheduling a (possibly failing) VolumeCreate.
// See warnUnmanagedVolumes for the accompanying user warning.
func (s *composeService) discoverUnmanagedVolumes(ctx context.Context, project *types.Project, state *ObservedState) error {
	for _, key := range project.VolumeNames() {
		vol := project.Volumes[key]
		if vol.External {
			continue
		}
		if _, ok := state.Volumes[key]; ok {
			continue
		}
		inspected, err := s.apiClient().VolumeInspect(ctx, vol.Name, client.VolumeInspectOptions{})
		if err != nil {
			if errdefs.IsNotFound(err) {
				continue // absent: it will be created by the reconciliation plan
			}
			return err
		}
		state.Volumes[key] = ObservedVolume{
			Name:        inspected.Volume.Name,
			ProjectName: inspected.Volume.Labels[api.ProjectLabel],
			Driver:      inspected.Volume.Driver,
			// ConfigHash intentionally left empty: the volume is not owned by
			// this project, so we must not treat it as diverged and recreate it.
		}
	}
	return nil
}

// toObservedContainer extracts the relevant fields from a container.Summary,
// parsing labels into typed values.
func toObservedContainer(c container.Summary) ObservedContainer {
	number, _ := strconv.Atoi(c.Labels[api.ContainerNumberLabel])

	networks := map[string]string{}
	if c.NetworkSettings != nil {
		for name, settings := range c.NetworkSettings.Networks {
			networks[name] = settings.NetworkID
		}
	}

	return ObservedContainer{
		ID:                c.ID,
		Name:              getCanonicalContainerName(c),
		State:             c.State,
		ConfigHash:        c.Labels[api.ConfigHashLabel],
		ImageDigest:       c.Labels[api.ImageDigestLabel],
		Number:            number,
		ConnectedNetworks: networks,
		Summary:           c,
	}
}

// setResolvedNetworks injects network IDs already resolved by ensureNetworks
// into the observed state, so the reconciler can compare container connections
// against actual network IDs.
func (s *ObservedState) setResolvedNetworks(networks map[string]string, project *types.Project) {
	for key, id := range networks {
		if obs, exists := s.Networks[key]; exists {
			obs.ID = id
			s.Networks[key] = obs
		} else {
			nw := project.Networks[key]
			s.Networks[key] = ObservedNetwork{ID: id, Name: nw.Name}
		}
	}
}

// setResolvedVolumes injects volume names already resolved by checkVolumes
// (external volumes) into the observed state. Managed volumes are discovered
// directly by collectObservedState, so only external ones need injecting.
func (s *ObservedState) setResolvedVolumes(volumes map[string]string) {
	for key, id := range volumes {
		if obs, exists := s.Volumes[key]; exists {
			obs.Name = id
			s.Volumes[key] = obs
		} else {
			s.Volumes[key] = ObservedVolume{Name: id}
		}
	}
}

// emitRunningEvents emits "Running" progress events for containers that are already
// running and have no operations planned for them. This matches the previous behavior
// where convergence.ensureService emitted runningEvent for up-to-date containers.
//
// Iterates project.Services (not observed.Containers) so that containers of
// disabled services (e.g. dependencies untouched by `compose run --no-deps`)
// are not falsely reported as Running — see issue 13882.
func emitRunningEvents(project *types.Project, observed *ObservedState, plan *Plan, events api.EventProcessor) {
	planned := map[string]bool{}
	for _, node := range plan.Nodes {
		if node.Operation.Container != nil {
			planned[node.Operation.Container.ID] = true
		}
	}

	for _, svc := range project.Services {
		for _, oc := range observed.Containers[svc.Name] {
			if oc.State == container.StateRunning && !planned[oc.ID] {
				events.On(newEvent("Container "+oc.Name, api.Done, api.StatusRunning))
			}
		}
	}
}

// orphanNames returns the names of orphaned containers as a comma-separated string.
func (s *ObservedState) orphanNames() string {
	names := make([]string, len(s.Orphans))
	for i, o := range s.Orphans {
		names[i] = o.Name
	}
	return strings.Join(names, ", ")
}

// containersByService flattens the observed containers into the shape
// resolveServiceReferences expects: project service name → raw Summaries.
func (s *ObservedState) containersByService() map[string]Containers {
	if s == nil {
		return map[string]Containers{}
	}
	result := make(map[string]Containers, len(s.Containers))
	for svc, ocs := range s.Containers {
		summaries := make(Containers, len(ocs))
		for i, oc := range ocs {
			summaries[i] = oc.Summary
		}
		result[svc] = summaries
	}
	return result
}
