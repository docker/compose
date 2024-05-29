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
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Ps(ctx context.Context, projectName string, options api.PsOptions) ([]api.ContainerSummary, error) {
	projectName = strings.ToLower(projectName)
	oneOff := oneOffExclude
	if options.All {
		oneOff = oneOffInclude
	}
	containers, err := s.getContainers(ctx, projectName, oneOff, options.All, options.Services...)
	if err != nil {
		return nil, err
	}

	if len(options.Services) != 0 {
		containers = containers.filter(isService(options.Services...))
	}
	summary := make([]api.ContainerSummary, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, container := range containers {
		i, container := i, container
		eg.Go(func() error {
			publishers := make([]api.PortPublisher, len(container.Ports))
			sort.Slice(container.Ports, func(i, j int) bool {
				return container.Ports[i].PrivatePort < container.Ports[j].PrivatePort
			})
			for i, p := range container.Ports {
				publishers[i] = api.PortPublisher{
					URL:           p.IP,
					TargetPort:    int(p.PrivatePort),
					PublishedPort: int(p.PublicPort),
					Protocol:      p.Type,
				}
			}

			inspect, err := s.apiClient().ContainerInspect(ctx, container.ID)
			if err != nil {
				return err
			}

			var (
				health   string
				exitCode int
			)
			if inspect.State != nil {
				switch inspect.State.Status {
				case "running":
					if inspect.State.Health != nil {
						health = inspect.State.Health.Status
					}
				case "exited", "dead":
					exitCode = inspect.State.ExitCode
				}
			}

			var (
				local  int
				mounts []string
			)
			for _, m := range container.Mounts {
				name := m.Name
				if name == "" {
					name = m.Source
				}
				if m.Driver == "local" {
					local++
				}
				mounts = append(mounts, name)
			}

			var networks []string
			if container.NetworkSettings != nil {
				for k := range container.NetworkSettings.Networks {
					networks = append(networks, k)
				}
			}

			summary[i] = api.ContainerSummary{
				ID:           container.ID,
				Name:         getCanonicalContainerName(container),
				Names:        container.Names,
				Image:        container.Image,
				Project:      container.Labels[api.ProjectLabel],
				Service:      container.Labels[api.ServiceLabel],
				Command:      container.Command,
				State:        container.State,
				Status:       container.Status,
				Created:      container.Created,
				Labels:       container.Labels,
				SizeRw:       container.SizeRw,
				SizeRootFs:   container.SizeRootFs,
				Mounts:       mounts,
				LocalVolumes: local,
				Networks:     networks,
				Health:       health,
				ExitCode:     exitCode,
				Publishers:   publishers,
			}
			return nil
		})
	}
	return summary, eg.Wait()
}
