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
	"strings"

	moby "github.com/docker/docker/api/types"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

func (s *composeService) Kill(ctx context.Context, projectName string, options api.KillOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.kill(ctx, strings.ToLower(projectName), options)
	})
}

func (s *composeService) kill(ctx context.Context, projectName string, options api.KillOptions) error {
	w := progress.ContextWriter(ctx)

	services := options.Services

	var containers Containers
	containers, err := s.getContainers(ctx, projectName, oneOffInclude, false, services...)
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
		fmt.Fprintf(s.stderr(), "no container to kill")
	}

	eg, ctx := errgroup.WithContext(ctx)
	containers.
		forEach(func(container moby.Container) {
			eg.Go(func() error {
				eventName := getContainerProgressName(container)
				w.Event(progress.KillingEvent(eventName))
				err := s.apiClient().ContainerKill(ctx, container.ID, options.Signal)
				if err != nil {
					w.Event(progress.ErrorMessageEvent(eventName, "Error while Killing"))
					return err
				}
				w.Event(progress.KilledEvent(eventName))
				return nil
			})
		})
	return eg.Wait()
}
