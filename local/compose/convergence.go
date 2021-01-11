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
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"golang.org/x/sync/errgroup"

	status "github.com/docker/compose-cli/local/moby"
	"github.com/docker/compose-cli/progress"
)

const (
	extLifecycle  = "x-lifecycle"
	forceRecreate = "force_recreate"

	doubledContainerNameWarning = "WARNING: The %q service is using the custom container name %q. " +
		"Docker requires each container to have a unique name. " +
		"Remove the custom name to scale the service.\n"
)

func (s *composeService) ensureScale(ctx context.Context, actual []moby.Container, scale int, project *types.Project, service types.ServiceConfig) (*errgroup.Group, []moby.Container, error) {
	eg, _ := errgroup.WithContext(ctx)
	if len(actual) < scale {
		next, err := nextContainerNumber(actual)
		if err != nil {
			return nil, actual, err
		}
		missing := scale - len(actual)
		for i := 0; i < missing; i++ {
			number := next + i
			name := getContainerName(project.Name, service, number)
			eg.Go(func() error {
				return s.createContainer(ctx, project, service, name, number, false)
			})
		}
	}

	if len(actual) > scale {
		for i := scale; i < len(actual); i++ {
			container := actual[i]
			eg.Go(func() error {
				err := s.apiClient.ContainerStop(ctx, container.ID, nil)
				if err != nil {
					return err
				}
				return s.apiClient.ContainerRemove(ctx, container.ID, moby.ContainerRemoveOptions{})
			})
		}
		actual = actual[:scale]
	}
	return eg, actual, nil
}

func (s *composeService) ensureService(ctx context.Context, observedState Containers, project *types.Project, service types.ServiceConfig) error {
	actual := observedState.filter(isService(service.Name))

	scale, err := getScale(service)
	if err != nil {
		return err
	}

	eg, actual, err := s.ensureScale(ctx, actual, scale, project, service)
	if err != nil {
		return err
	}

	expected, err := jsonHash(service)
	if err != nil {
		return err
	}

	for _, container := range actual {
		container := container
		name := getCanonicalContainerName(container)

		diverged := container.Labels[configHashLabel] != expected
		if diverged || service.Extensions[extLifecycle] == forceRecreate {
			eg.Go(func() error {
				return s.recreateContainer(ctx, project, service, container)
			})
			continue
		}

		w := progress.ContextWriter(ctx)
		switch container.State {
		case status.ContainerRunning:
			w.Event(progress.RunningEvent(name))
		case status.ContainerCreated:
		case status.ContainerRestarting:
			w.Event(progress.CreatedEvent(name))
		default:
			eg.Go(func() error {
				return s.restartContainer(ctx, container)
			})
		}
	}
	return eg.Wait()
}

func getContainerName(projectName string, service types.ServiceConfig, number int) string {
	name := fmt.Sprintf("%s_%s_%d", projectName, service.Name, number)
	if service.ContainerName != "" {
		name = service.ContainerName
	}
	return name
}

func (s *composeService) waitDependencies(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	eg, _ := errgroup.WithContext(ctx)
	for dep, config := range service.DependsOn {
		switch config.Condition {
		case "service_healthy":
			eg.Go(func() error {
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()
				for {
					<-ticker.C
					healthy, err := s.isServiceHealthy(ctx, project, dep)
					if err != nil {
						return err
					}
					if healthy {
						return nil
					}
				}
			})
		}
	}
	return eg.Wait()
}

func nextContainerNumber(containers []moby.Container) (int, error) {
	max := 0
	for _, c := range containers {
		n, err := strconv.Atoi(c.Labels[containerNumberLabel])
		if err != nil {
			return 0, err
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil

}

func getScale(config types.ServiceConfig) (int, error) {
	scale := 1
	var err error
	if config.Deploy != nil && config.Deploy.Replicas != nil {
		scale = int(*config.Deploy.Replicas)
	}
	if config.Scale != 0 {
		scale = config.Scale
	}
	if scale > 1 && config.ContainerName != "" {
		scale = -1
		err = fmt.Errorf(doubledContainerNameWarning,
			config.Name,
			config.ContainerName)
	}
	return scale, err
}

func (s *composeService) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name string, number int, autoRemove bool) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.CreatingEvent(name))
	err := s.createMobyContainer(ctx, project, service, name, number, nil, autoRemove)
	if err != nil {
		return err
	}
	w.Event(progress.CreatedEvent(name))
	return nil
}

func (s *composeService) recreateContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, container moby.Container) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(getCanonicalContainerName(container), progress.Working, "Recreate"))
	err := s.apiClient.ContainerStop(ctx, container.ID, nil)
	if err != nil {
		return err
	}
	name := getCanonicalContainerName(container)
	tmpName := fmt.Sprintf("%s_%s", container.ID[:12], name)
	err = s.apiClient.ContainerRename(ctx, container.ID, tmpName)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(container.Labels[containerNumberLabel])
	if err != nil {
		return err
	}
	err = s.createMobyContainer(ctx, project, service, name, number, &container, false)
	if err != nil {
		return err
	}
	err = s.apiClient.ContainerRemove(ctx, container.ID, moby.ContainerRemoveOptions{})
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(getCanonicalContainerName(container), progress.Done, "Recreated"))
	setDependentLifecycle(project, service.Name, forceRecreate)
	return nil
}

// setDependentLifecycle define the Lifecycle strategy for all services to depend on specified service
func setDependentLifecycle(project *types.Project, service string, strategy string) {
	for i, s := range project.Services {
		if contains(s.GetDependencies(), service) {
			if s.Extensions == nil {
				s.Extensions = map[string]interface{}{}
			}
			s.Extensions[extLifecycle] = strategy
			project.Services[i] = s
		}
	}
}

func (s *composeService) restartContainer(ctx context.Context, container moby.Container) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(getCanonicalContainerName(container), progress.Working, "Restart"))
	err := s.apiClient.ContainerStart(ctx, container.ID, moby.ContainerStartOptions{})
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(getCanonicalContainerName(container), progress.Done, "Restarted"))
	return nil
}

func (s *composeService) createMobyContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name string, number int, container *moby.Container, autoRemove bool) error {
	containerConfig, hostConfig, networkingConfig, err := s.getCreateOptions(ctx, project, service, number, container, autoRemove)
	if err != nil {
		return err
	}
	created, err := s.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, name)
	if err != nil {
		return err
	}
	id := created.ID
	for netName := range service.Networks {
		netwrk := project.Networks[netName]
		err = s.connectContainerToNetwork(ctx, id, netwrk.Name, service.Name, getContainerName(project.Name, service, number))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) connectContainerToNetwork(ctx context.Context, id string, netwrk string, aliases ...string) error {
	err := s.apiClient.NetworkConnect(ctx, netwrk, id, &network.EndpointSettings{
		Aliases: aliases,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *composeService) isServiceHealthy(ctx context.Context, project *types.Project, service string) (bool, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", projectLabel, project.Name)),
			filters.Arg("label", fmt.Sprintf("%s=%s", serviceLabel, service)),
		),
	})
	if err != nil {
		return false, err
	}

	for _, c := range containers {
		container, err := s.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, err
		}
		if container.State == nil || container.State.Health == nil {
			return false, fmt.Errorf("container for service %q has no healthcheck configured", service)
		}
		switch container.State.Health.Status {
		case "starting":
			return false, nil
		case "unhealthy":
			return false, nil
		}
	}
	return true, nil
}

func (s *composeService) startService(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	err := s.waitDependencies(ctx, project, service)
	if err != nil {
		return err
	}
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			serviceFilter(service.Name),
		),
		All: true,
	})
	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		container := c
		if container.State == status.ContainerRunning {
			continue
		}
		eg.Go(func() error {
			w := progress.ContextWriter(ctx)
			w.Event(progress.StartingEvent(getCanonicalContainerName(container)))
			err := s.apiClient.ContainerStart(ctx, container.ID, moby.ContainerStartOptions{})
			if err == nil {
				w.Event(progress.StartedEvent(getCanonicalContainerName(container)))
			}
			return err
		})
	}
	return eg.Wait()
}
