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

	"github.com/docker/compose-cli/api/compose"
)

func (s *composeService) Ps(ctx context.Context, projectName string, options compose.PsOptions) ([]compose.ContainerSummary, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
		),
		All: options.All,
	})
	if err != nil {
		return nil, err
	}

	var summary []compose.ContainerSummary
	for _, c := range containers {
		var publishers []compose.PortPublisher
		for _, p := range c.Ports {
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

		summary = append(summary, compose.ContainerSummary{
			ID:         c.ID,
			Name:       getCanonicalContainerName(c),
			Project:    c.Labels[projectLabel],
			Service:    c.Labels[serviceLabel],
			State:      c.State,
			Publishers: publishers,
		})
	}
	return summary, nil
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
