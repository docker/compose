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

	"github.com/compose-spec/compose-go/v2/types"
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
	raw, err := s.getContainers(ctx, project.Name, oneOffExclude, true)
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

	for _, c := range raw.filter(isNotOneOff) {
		oc := toObservedContainer(c)
		svcName := c.Labels[api.ServiceLabel]
		if knownServices[svcName] {
			state.Containers[svcName] = append(state.Containers[svcName], oc)
		} else {
			state.Orphans = append(state.Orphans, oc)
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

	return state, nil
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
