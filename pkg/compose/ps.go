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

	"github.com/moby/moby/api/types/container"
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
	for i, ctr := range containers {
		eg.Go(func() error {
			publishers := make([]api.PortPublisher, len(ctr.Ports))
			sort.Slice(ctr.Ports, func(i, j int) bool {
				return ctr.Ports[i].PrivatePort < ctr.Ports[j].PrivatePort
			})
			for i, p := range ctr.Ports {
				publishers[i] = api.PortPublisher{
					URL:           p.IP,
					TargetPort:    int(p.PrivatePort),
					PublishedPort: int(p.PublicPort),
					Protocol:      p.Type,
				}
			}

			inspect, err := s.apiClient().ContainerInspect(ctx, ctr.ID)
			if err != nil {
				return err
			}

			var (
				health   container.HealthStatus
				exitCode int
			)
			if inspect.State != nil {
				switch inspect.State.Status {
				case container.StateRunning:
					if inspect.State.Health != nil {
						health = inspect.State.Health.Status
					}
				case container.StateExited, container.StateDead:
					exitCode = inspect.State.ExitCode
				}
			}

			var (
				local  int
				mounts []string
			)
			for _, m := range ctr.Mounts {
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
			if ctr.NetworkSettings != nil {
				for k := range ctr.NetworkSettings.Networks {
					networks = append(networks, k)
				}
			}

			summary[i] = api.ContainerSummary{
				ID:           ctr.ID,
				Name:         getCanonicalContainerName(ctr),
				Names:        ctr.Names,
				Image:        ctr.Image,
				Project:      ctr.Labels[api.ProjectLabel],
				Service:      ctr.Labels[api.ServiceLabel],
				Command:      ctr.Command,
				State:        ctr.State,
				Status:       ctr.Status,
				Created:      ctr.Created,
				Labels:       ctr.Labels,
				SizeRw:       ctr.SizeRw,
				SizeRootFs:   ctr.SizeRootFs,
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
