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

	convert "github.com/docker/compose-cli/local/moby"

	"github.com/docker/compose-cli/api/compose"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) Ps(ctx context.Context, projectName string) ([]compose.ServiceStatus, error) {
	list, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
		),
	})
	if err != nil {
		return nil, err
	}
	return containersToServiceStatus(list)
}

func containersToServiceStatus(containers []moby.Container) ([]compose.ServiceStatus, error) {
	containersByLabel, keys, err := groupContainerByLabel(containers, serviceLabel)
	if err != nil {
		return nil, err
	}
	var services []compose.ServiceStatus
	for _, service := range keys {
		containers := containersByLabel[service]
		runnningContainers := []moby.Container{}
		for _, container := range containers {
			if container.State == convert.ContainerRunning {
				runnningContainers = append(runnningContainers, container)
			}
		}
		services = append(services, compose.ServiceStatus{
			ID:       service,
			Name:     service,
			Desired:  len(containers),
			Replicas: len(runnningContainers),
		})
	}
	return services, nil
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
