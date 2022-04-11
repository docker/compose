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

	"github.com/docker/compose/v2/pkg/api"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Restart(ctx context.Context, projectName string, options api.RestartOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.restart(ctx, strings.ToLower(projectName), options)
	})
}

func (s *composeService) restart(ctx context.Context, projectName string, options api.RestartOptions) error {
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

	if len(options.Services) == 0 {
		options.Services = project.ServiceNames()
	}

	w := progress.ContextWriter(ctx)
	return InDependencyOrder(ctx, project, func(c context.Context, service string) error {
		if !utils.StringContains(options.Services, service) {
			return nil
		}
		eg, ctx := errgroup.WithContext(ctx)
		for _, container := range containers.filter(isService(service)) {
			container := container
			eg.Go(func() error {
				eventName := getContainerProgressName(container)
				w.Event(progress.RestartingEvent(eventName))
				err := s.apiClient().ContainerRestart(ctx, container.ID, options.Timeout)
				if err == nil {
					w.Event(progress.StartedEvent(eventName))
				}
				return err
			})
		}
		return eg.Wait()
	})
}
