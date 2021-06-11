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
	"github.com/containerd/containerd/platforms"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"
	status "github.com/docker/compose-cli/local/moby"
	"github.com/docker/compose-cli/utils"
)

const (
	extLifecycle  = "x-lifecycle"
	forceRecreate = "force_recreate"

	doubledContainerNameWarning = "WARNING: The %q service is using the custom container name %q. " +
		"Docker requires each container to have a unique name. " +
		"Remove the custom name to scale the service.\n"
)

func (s *composeService) ensureScale(ctx context.Context, project *types.Project, service types.ServiceConfig, timeout *time.Duration) (*errgroup.Group, []moby.Container, error) {
	cState, err := GetContextContainerState(ctx)
	if err != nil {
		return nil, nil, err
	}
	observedState := cState.GetContainers()
	actual := observedState.filter(isService(service.Name)).filter(isNotOneOff)
	scale, err := getScale(service)
	if err != nil {
		return nil, nil, err
	}
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
				return s.createContainer(ctx, project, service, name, number, false, true)
			})
		}
	}

	if len(actual) > scale {
		for i := scale; i < len(actual); i++ {
			container := actual[i]
			eg.Go(func() error {
				err := s.apiClient.ContainerStop(ctx, container.ID, timeout)
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

func (s *composeService) ensureService(ctx context.Context, project *types.Project, service types.ServiceConfig, recreate string, inherit bool, timeout *time.Duration) error {
	eg, actual, err := s.ensureScale(ctx, project, service, timeout)
	if err != nil {
		return err
	}

	if recreate == compose.RecreateNever {
		return nil
	}

	expected, err := utils.ServiceHash(service)
	if err != nil {
		return err
	}

	for _, container := range actual {
		container := container
		name := getContainerProgressName(container)

		diverged := container.Labels[compose.ConfigHashLabel] != expected
		if diverged || recreate == compose.RecreateForce || service.Extensions[extLifecycle] == forceRecreate {
			eg.Go(func() error {
				return s.recreateContainer(ctx, project, service, container, inherit, timeout)
			})
			continue
		}

		w := progress.ContextWriter(ctx)
		switch container.State {
		case status.ContainerRunning:
			w.Event(progress.RunningEvent(name))
		case status.ContainerCreated:
		case status.ContainerRestarting:
		case status.ContainerExited:
			w.Event(progress.CreatedEvent(name))
		default:
			eg.Go(func() error {
				return s.startContainer(ctx, container)
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

func getContainerProgressName(container moby.Container) string {
	return "Container " + getCanonicalContainerName(container)
}

func (s *composeService) waitDependencies(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	eg, _ := errgroup.WithContext(ctx)
	for dep, config := range service.DependsOn {
		dep, config := dep, config
		eg.Go(func() error {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				<-ticker.C
				switch config.Condition {
				case types.ServiceConditionHealthy:
					healthy, err := s.isServiceHealthy(ctx, project, dep)
					if err != nil {
						return err
					}
					if healthy {
						return nil
					}
				case types.ServiceConditionCompletedSuccessfully:
					exited, code, err := s.isServiceCompleted(ctx, project, dep)
					if err != nil {
						return err
					}
					if exited {
						if code != 0 {
							return fmt.Errorf("service %q didn't completed successfully: exit %d", dep, code)
						}
						return nil
					}
				case types.ServiceConditionStarted:
					// already managed by InDependencyOrder
					return nil
				default:
					logrus.Warnf("unsupported depends_on condition: %s", config.Condition)
					return nil
				}
			}
		})
	}
	return eg.Wait()
}

func nextContainerNumber(containers []moby.Container) (int, error) {
	max := 0
	for _, c := range containers {
		n, err := strconv.Atoi(c.Labels[compose.ContainerNumberLabel])
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

func (s *composeService) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name string, number int, autoRemove bool, useNetworkAliases bool) error {
	w := progress.ContextWriter(ctx)
	eventName := "Container " + name
	w.Event(progress.CreatingEvent(eventName))
	err := s.createMobyContainer(ctx, project, service, name, number, nil, autoRemove, useNetworkAliases)
	if err != nil {
		return err
	}
	w.Event(progress.CreatedEvent(eventName))
	return nil
}

func (s *composeService) recreateContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, container moby.Container, inherit bool, timeout *time.Duration) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(getContainerProgressName(container), progress.Working, "Recreate"))
	err := s.apiClient.ContainerStop(ctx, container.ID, timeout)
	if err != nil {
		return err
	}
	name := getCanonicalContainerName(container)
	tmpName := fmt.Sprintf("%s_%s", container.ID[:12], name)
	err = s.apiClient.ContainerRename(ctx, container.ID, tmpName)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(container.Labels[compose.ContainerNumberLabel])
	if err != nil {
		return err
	}

	var inherited *moby.Container
	if inherit {
		inherited = &container
	}
	err = s.createMobyContainer(ctx, project, service, name, number, inherited, false, true)
	if err != nil {
		return err
	}
	err = s.apiClient.ContainerRemove(ctx, container.ID, moby.ContainerRemoveOptions{})
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(getContainerProgressName(container), progress.Done, "Recreated"))
	setDependentLifecycle(project, service.Name, forceRecreate)
	return nil
}

// setDependentLifecycle define the Lifecycle strategy for all services to depend on specified service
func setDependentLifecycle(project *types.Project, service string, strategy string) {
	for i, s := range project.Services {
		if utils.StringContains(s.GetDependencies(), service) {
			if s.Extensions == nil {
				s.Extensions = map[string]interface{}{}
			}
			s.Extensions[extLifecycle] = strategy
			project.Services[i] = s
		}
	}
}

func (s *composeService) startContainer(ctx context.Context, container moby.Container) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(getContainerProgressName(container), progress.Working, "Restart"))
	err := s.apiClient.ContainerStart(ctx, container.ID, moby.ContainerStartOptions{})
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(getContainerProgressName(container), progress.Done, "Restarted"))
	return nil
}

func (s *composeService) createMobyContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name string, number int,
	inherit *moby.Container,
	autoRemove bool,
	useNetworkAliases bool) error {
	cState, err := GetContextContainerState(ctx)
	if err != nil {
		return err
	}
	containerConfig, hostConfig, networkingConfig, err := s.getCreateOptions(ctx, project, service, number, inherit, autoRemove)
	if err != nil {
		return err
	}
	var plat *specs.Platform
	if service.Platform != "" {
		p, err := platforms.Parse(service.Platform)
		if err != nil {
			return err
		}
		plat = &p
	}
	created, err := s.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, plat, name)
	if err != nil {
		return err
	}
	createdContainer := moby.Container{
		ID:     created.ID,
		Labels: containerConfig.Labels,
	}
	cState.Add(createdContainer)
	for _, netName := range service.NetworksByPriority() {
		netwrk := project.Networks[netName]
		cfg := service.Networks[netName]
		aliases := []string{getContainerName(project.Name, service, number)}
		if useNetworkAliases {
			aliases = append(aliases, service.Name)
			if cfg != nil {
				aliases = append(aliases, cfg.Aliases...)
			}
		}

		err = s.connectContainerToNetwork(ctx, created.ID, netwrk.Name, cfg, aliases...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) connectContainerToNetwork(ctx context.Context, id string, netwrk string, cfg *types.ServiceNetworkConfig, aliases ...string) error {
	var (
		ipv4ddress  string
		ipv6Address string
	)
	if cfg != nil {
		ipv4ddress = cfg.Ipv4Address
		ipv6Address = cfg.Ipv6Address
	}
	err := s.apiClient.NetworkConnect(ctx, netwrk, id, &network.EndpointSettings{
		Aliases:           aliases,
		IPAddress:         ipv4ddress,
		GlobalIPv6Address: ipv6Address,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *composeService) isServiceHealthy(ctx context.Context, project *types.Project, service string) (bool, error) {
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, false, service)
	if err != nil {
		return false, err
	}

	if len(containers) == 0 {
		return false, nil
	}
	for _, c := range containers {
		container, err := s.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, err
		}
		if container.State == nil || container.State.Health == nil {
			return false, fmt.Errorf("container for service %q has no healthcheck configured", service)
		}
		if container.State.Health.Status != moby.Healthy {
			return false, nil
		}
	}
	return true, nil
}

func (s *composeService) isServiceCompleted(ctx context.Context, project *types.Project, dep string) (bool, int, error) {
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, true, dep)
	if err != nil {
		return false, 0, err
	}
	for _, c := range containers {
		container, err := s.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, 0, err
		}
		if container.State != nil && container.State.Status == "exited" {
			return true, container.State.ExitCode, nil
		}
	}
	return false, 0, nil
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

	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		container := c
		if container.State == status.ContainerRunning {
			continue
		}
		eg.Go(func() error {
			eventName := getContainerProgressName(container)
			w.Event(progress.StartingEvent(eventName))
			err := s.apiClient.ContainerStart(ctx, container.ID, moby.ContainerStartOptions{})
			if err == nil {
				w.Event(progress.StartedEvent(eventName))
			}
			return err
		})
	}
	return eg.Wait()
}

func (s *composeService) restartService(ctx context.Context, serviceName string, timeout *time.Duration) error {
	containerState, err := GetContextContainerState(ctx)
	if err != nil {
		return err
	}
	containers := containerState.GetContainers().filter(isService(serviceName))
	w := progress.ContextWriter(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		container := c
		eg.Go(func() error {
			eventName := getContainerProgressName(container)
			w.Event(progress.RestartingEvent(eventName))
			err := s.apiClient.ContainerRestart(ctx, container.ID, timeout)
			if err == nil {
				w.Event(progress.StartedEvent(eventName))
			}
			return err
		})
	}
	return eg.Wait()
}
