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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

// ObservedState captures the current state of a Compose project as seen by the Docker engine.
type ObservedState struct {
	ProjectName string
	Containers  map[string]Containers
	Networks    map[string]ObservedNetwork
	Volumes     map[string]ObservedVolume
	Orphans     Containers
}

// ObservedNetwork represents a Docker network associated with a Compose project.
type ObservedNetwork struct {
	ID         string
	Name       string
	Driver     string
	Labels     map[string]string
	ConfigHash string
}

// ObservedVolume represents a Docker volume associated with a Compose project.
type ObservedVolume struct {
	Name       string
	Driver     string
	Labels     map[string]string
	ConfigHash string
}

// InspectState queries the Docker engine to build an ObservedState for the given project.
func (s *composeService) InspectState(ctx context.Context, project *types.Project) (*ObservedState, error) {
	var (
		allContainers Containers
		networks      client.NetworkListResult
		volumes       client.VolumeListResult
	)

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		var err error
		allContainers, err = s.getContainers(ctx, project.Name, oneOffInclude, true)
		return err
	})

	eg.Go(func() error {
		var err error
		networks, err = s.apiClient().NetworkList(ctx, client.NetworkListOptions{
			Filters: projectFilter(project.Name),
		})
		return err
	})

	eg.Go(func() error {
		var err error
		volumes, err = s.apiClient().VolumeList(ctx, client.VolumeListOptions{
			Filters: projectFilter(project.Name),
		})
		return err
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Partition containers by service
	containersByService := map[string]Containers{}
	for _, c := range allContainers {
		service := c.Labels[api.ServiceLabel]
		containersByService[service] = append(containersByService[service], c)
	}

	// Identify orphan containers
	orphans := allContainers.filter(isOrphaned(project))

	// Map networks by their Compose network name
	observedNetworks := map[string]ObservedNetwork{}
	for _, n := range networks.Items {
		name := n.Labels[api.NetworkLabel]
		observedNetworks[name] = ObservedNetwork{
			ID:         n.ID,
			Name:       n.Name,
			Driver:     n.Driver,
			Labels:     n.Labels,
			ConfigHash: n.Labels[api.ConfigHashLabel],
		}
	}

	// Map volumes by their Compose volume name
	observedVolumes := map[string]ObservedVolume{}
	for _, v := range volumes.Items {
		name := v.Labels[api.VolumeLabel]
		observedVolumes[name] = ObservedVolume{
			Name:       v.Name,
			Driver:     v.Driver,
			Labels:     v.Labels,
			ConfigHash: v.Labels[api.ConfigHashLabel],
		}
	}

	return &ObservedState{
		ProjectName: project.Name,
		Containers:  containersByService,
		Networks:    observedNetworks,
		Volumes:     observedVolumes,
		Orphans:     orphans,
	}, nil
}
