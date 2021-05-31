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

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/utils"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	listener := options.Attach
	if len(options.Services) == 0 {
		options.Services = project.ServiceNames()
	}

	eg, ctx := errgroup.WithContext(ctx)
	if listener != nil {
		attached, err := s.attach(ctx, project, listener, options.Services)
		if err != nil {
			return err
		}

		eg.Go(func() error {
			return s.watchContainers(project, options.Services, listener, attached)
		})
	}

	err := InDependencyOrder(ctx, project, func(c context.Context, service types.ServiceConfig) error {
		if utils.StringContains(options.Services, service.Name) {
			return s.startService(ctx, project, service)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return eg.Wait()
}

// watchContainers uses engine events to capture container start/die and notify ContainerEventListener
func (s *composeService) watchContainers(project *types.Project, services []string, listener compose.ContainerEventListener, containers Containers) error {
	watched := map[string]int{}
	for _, c := range containers {
		watched[c.ID] = 0
	}

	ctx, stop := context.WithCancel(context.Background())
	err := s.Events(ctx, project.Name, compose.EventsOptions{
		Services: services,
		Consumer: func(event compose.Event) error {
			inspected, err := s.apiClient.ContainerInspect(ctx, event.Container)
			if err != nil {
				return err
			}
			container := moby.Container{
				ID:     inspected.ID,
				Names:  []string{inspected.Name},
				Labels: inspected.Config.Labels,
			}
			name := getContainerNameWithoutProject(container)

			if event.Status == "die" {
				restarted := watched[container.ID]
				watched[container.ID] = restarted + 1
				// Container terminated.
				willRestart := inspected.HostConfig.RestartPolicy.MaximumRetryCount > restarted

				listener(compose.ContainerEvent{
					Type:       compose.ContainerEventExit,
					Container:  name,
					Service:    container.Labels[serviceLabel],
					ExitCode:   inspected.State.ExitCode,
					Restarting: willRestart,
				})

				if !willRestart {
					// we're done with this one
					delete(watched, container.ID)
				}

				if len(watched) == 0 {
					// all project containers stopped, we're done
					stop()
				}
				return nil
			}

			if event.Status == "start" {
				count, ok := watched[container.ID]
				mustAttach := ok && count > 0 // Container restarted, need to re-attach
				if !ok {
					// A new container has just been added to service by scale
					watched[container.ID] = 0
					mustAttach = true
				}
				if mustAttach {
					// Container restarted, need to re-attach
					err := s.attachContainer(ctx, container, listener, project)
					if err != nil {
						return err
					}
				}
			}

			return nil
		},
	})
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return err
}
