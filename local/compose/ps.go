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
	"sort"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose-cli/api/compose"
)

func (s *composeService) Ps(ctx context.Context, projectName string, options compose.PsOptions) ([]compose.ContainerSummary, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
		All:     options.All,
	})
	if err != nil {
		return nil, err
	}

	summary := make([]compose.ContainerSummary, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, c := range containers {
		container := c
		i := i
		eg.Go(func() error {
			var publishers []compose.PortPublisher
			for _, p := range container.Ports {
				var url string
				if p.PublicPort != 0 {
					url = fmt.Sprintf("%s:%d", p.IP, p.PublicPort)
				}
				publishers = append(publishers, compose.PortPublisher{
					URL:           url,
					TargetPort:    int(p.PrivatePort),
					PublishedPort: int(p.PublicPort),
					Protocol:      p.Type,
				})
			}

			inspect, err := s.apiClient.ContainerInspect(ctx, container.ID)
			if err != nil {
				return err
			}

			var health string
			if inspect.State != nil && inspect.State.Health != nil {
				health = inspect.State.Health.Status
			}

			summary[i] = compose.ContainerSummary{
				ID:         container.ID,
				Name:       getCanonicalContainerName(container),
				Project:    container.Labels[projectLabel],
				Service:    container.Labels[serviceLabel],
				State:      container.State,
				Health:     health,
				Publishers: publishers,
			}
			return nil
		})
	}
	return summary, eg.Wait()
}

func groupContainerByLabel(containers []moby.Container, labelName string) (map[string][]moby.Container, []string, error) {
	containersByLabel := map[string][]moby.Container{}
	keys := []string{}
	for _, c := range containers {
		label, ok := c.Labels[labelName]
		if !ok {
			return nil, nil, fmt.Errorf("No label %q set on container %q of compose project", labelName, c.ID)
		}
		labelContainers, ok := containersByLabel[label]
		if !ok {
			labelContainers = []moby.Container{}
			keys = append(keys, label)
		}
		labelContainers = append(labelContainers, c)
		containersByLabel[label] = labelContainers
	}
	sort.Strings(keys)
	return containersByLabel, keys, nil
}
