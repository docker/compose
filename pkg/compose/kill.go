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

	"github.com/docker/docker/api/types/container"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Kill(ctx context.Context, projectName string, options api.KillOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return s.kill(ctx, strings.ToLower(projectName), options)
	}, "kill", s.events)
}

func (s *composeService) kill(ctx context.Context, projectName string, options api.KillOptions) error {
	services := options.Services

	var containers Containers
	containers, err := s.getContainers(ctx, projectName, oneOffInclude, options.All, services...)
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

	if !options.RemoveOrphans {
		containers = containers.filter(isService(project.ServiceNames()...))
	}
	if len(containers) == 0 {
		return api.ErrNoResources
	}

	eg, ctx := errgroup.WithContext(ctx)
	containers.forEach(func(ctr container.Summary) {
		eg.Go(func() error {
			eventName := getContainerProgressName(ctr)
			s.events.On(killingEvent(eventName))
			err := s.apiClient().ContainerKill(ctx, ctr.ID, options.Signal)
			if err != nil {
				s.events.On(errorEvent(eventName, "Error while Killing"))
				return err
			}
			s.events.On(killedEvent(eventName))
			return nil
		})
	})
	return eg.Wait()
}
