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
	convert "github.com/docker/compose-cli/local/moby"
	"github.com/docker/compose-cli/utils"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	if len(options.Services) == 0 {
		options.Services = project.ServiceNames()
	}

	var containers Containers
	if options.Attach != nil {
		attached, err := s.attach(ctx, project, options.Attach, options.Services)
		if err != nil {
			return err
		}
		containers = attached

		// Watch events to capture container restart and re-attach
		go func() {
			watched := map[string]struct{}{}
			for _, c := range containers {
				watched[c.ID] = struct{}{}
			}
			s.Events(ctx, project.Name, compose.EventsOptions{ // nolint: errcheck
				Services: options.Services,
				Consumer: func(event compose.Event) error {
					if event.Status == "start" {
						inspect, err := s.apiClient.ContainerInspect(ctx, event.Container)
						if err != nil {
							return err
						}

						container := moby.Container{
							ID:    event.Container,
							Names: []string{inspect.Name},
							State: convert.ContainerRunning,
							Labels: map[string]string{
								projectLabel: project.Name,
								serviceLabel: event.Service,
							},
						}

						// Just ignore errors when reattaching to already crashed containers
						s.attachContainer(ctx, container, options.Attach, project) // nolint: errcheck

						if _, ok := watched[inspect.ID]; !ok {
							// a container has been added to the service, see --scale option
							watched[inspect.ID] = struct{}{}
							go func() {
								s.waitContainer(container, options.Attach) // nolint: errcheck
							}()
						}
					}
					return nil
				},
			})
		}()
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

	if options.Attach == nil {
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		c := c
		eg.Go(func() error {
			return s.waitContainer(c, options.Attach)
		})
	}
	return eg.Wait()
}

func (s *composeService) waitContainer(c moby.Container, listener compose.ContainerEventListener) error {
	statusC, errC := s.apiClient.ContainerWait(context.Background(), c.ID, container.WaitConditionNotRunning)
	name := getContainerNameWithoutProject(c)
	select {
	case status := <-statusC:
		listener(compose.ContainerEvent{
			Type:      compose.ContainerEventExit,
			Container: name,
			Service:   c.Labels[serviceLabel],
			ExitCode:  int(status.StatusCode),
		})
		return nil
	case err := <-errC:
		return err
	}
}
