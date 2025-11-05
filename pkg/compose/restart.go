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
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	"github.com/docker/docker/api/types/container"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.restart(ctx, strings.ToLower(projectName), options)
	}, "restart", s.events)
}

func (s *composeService) restart(ctx context.Context, projectName string, options api.RestartOptions) error { //nolint:gocyclo
	containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
	if err != nil {
		return err
	}

	project := options.Project
	if project == nil {
		project, err = s.getProjectWithResources(ctx, containers, projectName)
		if err != nil {
			return err
		}
	}

	if options.NoDeps {
		project, err = project.WithSelectedServices(options.Services, types.IgnoreDependencies)
		if err != nil {
			return err
		}
	}

	// ignore depends_on relations which are not impacted by restarting service or not required
	project, err = project.WithServicesTransform(func(_ string, s types.ServiceConfig) (types.ServiceConfig, error) {
		for name, r := range s.DependsOn {
			if !r.Restart {
				delete(s.DependsOn, name)
			}
		}
		return s, nil
	})
	if err != nil {
		return err
	}

	if len(options.Services) != 0 {
		project, err = project.WithSelectedServices(options.Services, types.IncludeDependents)
		if err != nil {
			return err
		}
	}

	return InDependencyOrder(ctx, project, func(c context.Context, service string) error {
		config := project.Services[service]
		err = s.waitDependencies(ctx, project, service, config.DependsOn, containers, 0)
		if err != nil {
			return err
		}

		eg, ctx := errgroup.WithContext(ctx)
		for _, ctr := range containers.filter(isService(service)) {
			eg.Go(func() error {
				def := project.Services[service]
				for _, hook := range def.PreStop {
					err = s.runHook(ctx, ctr, def, hook, nil)
					if err != nil {
						return err
					}
				}
				eventName := getContainerProgressName(ctr)
				s.events.On(restartingEvent(eventName))
				timeout := utils.DurationSecondToInt(options.Timeout)
				err = s.apiClient().ContainerRestart(ctx, ctr.ID, container.StopOptions{Timeout: timeout})
				if err != nil {
					return err
				}
				s.events.On(startedEvent(eventName))
				for _, hook := range def.PostStart {
					err = s.runHook(ctx, ctr, def, hook, nil)
					if err != nil {
						return err
					}
				}
				return nil
			})
		}
		return eg.Wait()
	})
}
