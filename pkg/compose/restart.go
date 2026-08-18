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
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

func (s *composeService) Restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.restart(ctx, strings.ToLower(projectName), options)
	}, "restart", s.events)
}

func (s *composeService) restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	containers, err := s.getContainers(ctx, projectName, oneOffExclude, true)
	if err != nil {
		return err
	}

	project, err := s.prepareRestartProject(ctx, containers, projectName, options)
	if err != nil {
		return err
	}

	return InDependencyOrder(ctx, project, func(c context.Context, service string) error {
		config := project.Services[service]
		err := s.waitDependencies(ctx, project, service, config.DependsOn, containers, 0)
		if err != nil {
			return err
		}

		eg, ctx := errgroup.WithContext(ctx)
		for _, ctr := range containers.filter(isService(service)) {
			eg.Go(func() error {
				return s.restartContainer(ctx, project.Services[service], ctr, options)
			})
		}
		return eg.Wait()
	})
}

// prepareRestartProject resolves the project restart applies to, restricted
// to the requested services and the depends_on relations with restart: true
func (s *composeService) prepareRestartProject(ctx context.Context, containers Containers, projectName string, options api.RestartOptions) (*types.Project, error) {
	project := options.Project
	var err error
	if project == nil {
		project, err = s.getProjectWithResources(ctx, containers, projectName)
		if err != nil {
			return nil, err
		}
	}

	if options.NoDeps {
		project, err = project.WithSelectedServices(options.Services, types.IgnoreDependencies)
		if err != nil {
			return nil, err
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
		return nil, err
	}

	if len(options.Services) != 0 {
		project, err = project.WithSelectedServices(options.Services, types.IncludeDependents)
		if err != nil {
			return nil, err
		}
	}
	return project, nil
}

// restartContainer restarts a container, running its pre_stop and post_start
// hooks around the restart
func (s *composeService) restartContainer(ctx context.Context, def types.ServiceConfig, ctr container.Summary, options api.RestartOptions) error {
	for _, hook := range def.PreStop {
		err := s.runHook(ctx, ctr, def, hook, nil)
		if err != nil {
			return err
		}
	}
	eventName := getContainerProgressName(ctr)
	s.events.On(newEvent(eventName, api.Working, api.StatusRestarting))
	_, err := s.apiClient().ContainerRestart(ctx, ctr.ID, client.ContainerRestartOptions{
		Timeout: utils.DurationSecondToInt(options.Timeout),
	})
	if err != nil {
		return err
	}
	s.events.On(newEvent(eventName, api.Done, api.StatusStarted))
	for _, hook := range def.PostStart {
		err := s.runHook(ctx, ctr, def, hook, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
