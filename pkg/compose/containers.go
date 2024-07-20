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
	"strconv"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// Containers is a set of moby Container
type Containers []moby.Container

type oneOff int

const (
	oneOffInclude = oneOff(iota)
	oneOffExclude
	oneOffOnly
)

func (s *composeService) getContainers(ctx context.Context, project string, oneOff oneOff, all bool, selectedServices ...string) (Containers, error) {
	var containers Containers
	f := getDefaultFilters(project, oneOff, selectedServices...)
	containers, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filters.NewArgs(f...),
		All:     all,
	})
	if err != nil {
		return nil, err
	}
	if len(selectedServices) > 1 {
		containers = containers.filter(isService(selectedServices...))
	}
	return containers, nil
}

func getDefaultFilters(projectName string, oneOff oneOff, selectedServices ...string) []filters.KeyValuePair {
	f := []filters.KeyValuePair{projectFilter(projectName)}
	if len(selectedServices) == 1 {
		f = append(f, serviceFilter(selectedServices[0]))
	}
	f = append(f, hasConfigHashLabel())
	switch oneOff {
	case oneOffOnly:
		f = append(f, oneOffFilter(true))
	case oneOffExclude:
		f = append(f, oneOffFilter(false))
	case oneOffInclude:
	}
	return f
}

func (s *composeService) getSpecifiedContainer(ctx context.Context, projectName string, oneOff oneOff, all bool, serviceName string, containerIndex int) (moby.Container, error) {
	defaultFilters := getDefaultFilters(projectName, oneOff, serviceName)
	if containerIndex > 0 {
		defaultFilters = append(defaultFilters, containerNumberFilter(containerIndex))
	}
	containers, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filters.NewArgs(
			defaultFilters...,
		),
		All: all,
	})
	if err != nil {
		return moby.Container{}, err
	}
	if len(containers) < 1 {
		if containerIndex > 0 {
			return moby.Container{}, fmt.Errorf("service %q is not running container #%d", serviceName, containerIndex)
		}
		return moby.Container{}, fmt.Errorf("service %q is not running", serviceName)
	}

	// Sort by container number first, then put one-off containers at the end
	sort.Slice(containers, func(i, j int) bool {
		numberLabelX, _ := strconv.Atoi(containers[i].Labels[api.ContainerNumberLabel])
		numberLabelY, _ := strconv.Atoi(containers[j].Labels[api.ContainerNumberLabel])
		IsOneOffLabelTrueX := containers[i].Labels[api.OneoffLabel] == "True"
		IsOneOffLabelTrueY := containers[j].Labels[api.OneoffLabel] == "True"

		if numberLabelX == numberLabelY {
			return !IsOneOffLabelTrueX && IsOneOffLabelTrueY
		}

		return numberLabelX < numberLabelY
	})
	container := containers[0]
	return container, nil
}

// containerPredicate define a predicate we want container to satisfy for filtering operations
type containerPredicate func(c moby.Container) bool

func isService(services ...string) containerPredicate {
	return func(c moby.Container) bool {
		service := c.Labels[api.ServiceLabel]
		return utils.StringContains(services, service)
	}
}

func isRunning() containerPredicate {
	return func(c moby.Container) bool {
		return c.State == "running"
	}
}

// isOrphaned is a predicate to select containers without a matching service definition in compose project
func isOrphaned(project *types.Project) containerPredicate {
	services := append(project.ServiceNames(), project.DisabledServiceNames()...)
	return func(c moby.Container) bool {
		// One-off container
		v, ok := c.Labels[api.OneoffLabel]
		if ok && v == "True" {
			return c.State == ContainerExited || c.State == ContainerDead
		}
		// Service that is not defined in the compose model
		service := c.Labels[api.ServiceLabel]
		return !utils.StringContains(services, service)
	}
}

func isNotOneOff(c moby.Container) bool {
	v, ok := c.Labels[api.OneoffLabel]
	return !ok || v == "False"
}

// filter return Containers with elements to match predicate
func (containers Containers) filter(predicate containerPredicate) Containers {
	var filtered Containers
	for _, c := range containers {
		if predicate(c) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func (containers Containers) names() []string {
	var names []string
	for _, c := range containers {
		names = append(names, getCanonicalContainerName(c))
	}
	return names
}

func (containers Containers) forEach(fn func(moby.Container)) {
	for _, c := range containers {
		fn(c)
	}
}

func (containers Containers) sorted() Containers {
	sort.Slice(containers, func(i, j int) bool {
		return getCanonicalContainerName(containers[i]) < getCanonicalContainerName(containers[j])
	})
	return containers
}
