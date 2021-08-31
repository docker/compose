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

	"github.com/docker/compose/v2/pkg/api"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) List(ctx context.Context, opts api.ListOptions) ([]api.Stack, error) {
	list, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(hasProjectLabelFilter()),
		All:     opts.All,
	})
	if err != nil {
		return nil, err
	}

	return containersToStacks(list)
}

func containersToStacks(containers []moby.Container) ([]api.Stack, error) {
	containersByLabel, keys, err := groupContainerByLabel(containers, api.ProjectLabel)
	if err != nil {
		return nil, err
	}
	var projects []api.Stack
	for _, project := range keys {
		projects = append(projects, api.Stack{
			ID:     project,
			Name:   project,
			Status: combinedStatus(containerToState(containersByLabel[project])),
		})
	}
	return projects, nil
}

func containerToState(containers []moby.Container) []string {
	statuses := []string{}
	for _, c := range containers {
		statuses = append(statuses, c.State)
	}
	return statuses
}

func combinedStatus(statuses []string) string {
	nbByStatus := map[string]int{}
	keys := []string{}
	for _, status := range statuses {
		nb, ok := nbByStatus[status]
		if !ok {
			nb = 0
			keys = append(keys, status)
		}
		nbByStatus[status] = nb + 1
	}
	sort.Strings(keys)
	result := ""
	for _, status := range keys {
		nb := nbByStatus[status]
		if result != "" {
			result = result + ", "
		}
		result = result + fmt.Sprintf("%s(%d)", status, nb)
	}
	return result
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
