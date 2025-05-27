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
	"slices"
	"sort"
	"strings"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/sirupsen/logrus"
)

func (s *composeService) List(ctx context.Context, opts api.ListOptions) ([]api.Stack, error) {
	list, err := s.apiClient().ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(hasProjectLabelFilter(), hasConfigHashLabel()),
		All:     opts.All,
	})
	if err != nil {
		return nil, err
	}

	return containersToStacks(list)
}

func containersToStacks(containers []container.Summary) ([]api.Stack, error) {
	containersByLabel, keys, err := groupContainerByLabel(containers, api.ProjectLabel)
	if err != nil {
		return nil, err
	}
	var projects []api.Stack
	for _, project := range keys {
		configFiles, err := combinedConfigFiles(containersByLabel[project])
		if err != nil {
			logrus.Warn(err.Error())
			configFiles = "N/A"
		}

		projects = append(projects, api.Stack{
			ID:          project,
			Name:        project,
			Status:      combinedStatus(containerToState(containersByLabel[project])),
			ConfigFiles: configFiles,
		})
	}
	return projects, nil
}

func combinedConfigFiles(containers []container.Summary) (string, error) {
	configFiles := []string{}

	for _, c := range containers {
		files, ok := c.Labels[api.ConfigFilesLabel]
		if !ok {
			return "", fmt.Errorf("no label %q set on container %q of compose project", api.ConfigFilesLabel, c.ID)
		}

		for _, f := range strings.Split(files, ",") {
			if !slices.Contains(configFiles, f) {
				configFiles = append(configFiles, f)
			}
		}
	}

	return strings.Join(configFiles, ","), nil
}

func containerToState(containers []container.Summary) []string {
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
			result += ", "
		}
		result += fmt.Sprintf("%s(%d)", status, nb)
	}
	return result
}

func groupContainerByLabel(containers []container.Summary, labelName string) (map[string][]container.Summary, []string, error) {
	containersByLabel := map[string][]container.Summary{}
	keys := []string{}
	for _, c := range containers {
		label, ok := c.Labels[labelName]
		if !ok {
			return nil, nil, fmt.Errorf("no label %q set on container %q of compose project", labelName, c.ID)
		}
		labelContainers, ok := containersByLabel[label]
		if !ok {
			labelContainers = []container.Summary{}
			keys = append(keys, label)
		}
		labelContainers = append(labelContainers, c)
		containersByLabel[label] = labelContainers
	}
	sort.Strings(keys)
	return containersByLabel, keys, nil
}
