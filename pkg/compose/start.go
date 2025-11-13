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
	"errors"
	"fmt"
	"strings"

	"github.com/docker/compose/v5/pkg/api"
	containerType "github.com/docker/docker/api/types/container"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) Start(ctx context.Context, projectName string, options api.StartOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.start(ctx, strings.ToLower(projectName), options, nil)
	}, "start", s.events)
}

func (s *composeService) start(ctx context.Context, projectName string, options api.StartOptions, listener api.ContainerEventListener) error {
	project := options.Project
	if project == nil {
		var containers Containers
		containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
		if err != nil {
			return err
		}

		project, err = s.projectFromName(containers, projectName, options.AttachTo...)
		if err != nil {
			return err
		}
	}

	var containers Containers
	containers, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			oneOffFilter(false),
		),
		All: true,
	})
	if err != nil {
		return err
	}

	err = InDependencyOrder(ctx, project, func(c context.Context, name string) error {
		service, err := project.GetService(name)
		if err != nil {
			return err
		}

		return s.startService(ctx, project, service, containers, listener, options.WaitTimeout)
	})
	if err != nil {
		return err
	}

	if options.Wait {
		depends := types.DependsOnConfig{}
		for _, s := range project.Services {
			depends[s.Name] = types.ServiceDependency{
				Condition: getDependencyCondition(s, project),
				Required:  true,
			}
		}
		if options.WaitTimeout > 0 {
			withTimeout, cancel := context.WithTimeout(ctx, options.WaitTimeout)
			ctx = withTimeout
			defer cancel()
		}

		err = s.waitDependencies(ctx, project, project.Name, depends, containers, 0)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("application not healthy after %s", options.WaitTimeout)
			}
			return err
		}
	}

	return nil
}

// getDependencyCondition checks if service is depended on by other services
// with service_completed_successfully condition, and applies that condition
// instead, or --wait will never finish waiting for one-shot containers
func getDependencyCondition(service types.ServiceConfig, project *types.Project) string {
	for _, services := range project.Services {
		for dependencyService, dependencyConfig := range services.DependsOn {
			if dependencyService == service.Name && dependencyConfig.Condition == types.ServiceConditionCompletedSuccessfully {
				return types.ServiceConditionCompletedSuccessfully
			}
		}
	}
	return ServiceConditionRunningOrHealthy
}
