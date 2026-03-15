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
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/platforms"
	"github.com/moby/moby/api/types/container"
	mmount "github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

const (
	doubledContainerNameWarning = "WARNING: The %q service is using the custom container name %q. " +
		"Docker requires each container to have a unique name. " +
		"Remove the custom name to scale the service"
)

func getScale(config types.ServiceConfig) (int, error) {
	scale := config.GetScale()
	if scale > 1 && config.ContainerName != "" {
		return 0, fmt.Errorf(doubledContainerNameWarning,
			config.Name,
			config.ContainerName)
	}
	return scale, nil
}

func checkExpectedNetworks(expected types.ServiceConfig, actual container.Summary, networks map[string]string) bool {
	// check the networks container is connected to are the expected ones
	for net := range expected.Networks {
		id := networks[net]
		if id == "swarm" {
			// corner-case : swarm overlay network isn't visible until a container is attached
			continue
		}
		found := false
		for _, settings := range actual.NetworkSettings.Networks {
			if settings.NetworkID == id {
				found = true
				break
			}
		}
		if !found {
			// config is up-to-date but container is not connected to network
			return true
		}
	}
	return false
}

func checkExpectedVolumes(expected types.ServiceConfig, actual container.Summary, volumes map[string]string) bool {
	// check container's volume mounts and search for the expected ones
	for _, vol := range expected.Volumes {
		if vol.Type != string(mmount.TypeVolume) {
			continue
		}
		if vol.Source == "" {
			continue
		}
		id := volumes[vol.Source]
		found := false
		for _, mount := range actual.Mounts {
			if mount.Type != mmount.TypeVolume {
				continue
			}
			if mount.Name == id {
				found = true
				break
			}
		}
		if !found {
			// config is up-to-date but container doesn't have volume mounted
			return true
		}
	}
	return false
}

func getContainerName(projectName string, service types.ServiceConfig, number int) string {
	name := getDefaultContainerName(projectName, service.Name, strconv.Itoa(number))
	if service.ContainerName != "" {
		name = service.ContainerName
	}
	return name
}

func getDefaultContainerName(projectName, serviceName, index string) string {
	return strings.Join([]string{projectName, serviceName, index}, api.Separator)
}

func getContainerProgressName(ctr container.Summary) string {
	return "Container " + getCanonicalContainerName(ctr)
}

func containerEvents(containers Containers, eventFunc func(string) api.Resource) []api.Resource {
	events := []api.Resource{}
	for _, ctr := range containers {
		events = append(events, eventFunc(getContainerProgressName(ctr)))
	}
	return events
}

func containerReasonEvents(containers Containers, eventFunc func(string, string) api.Resource, reason string) []api.Resource {
	events := []api.Resource{}
	for _, ctr := range containers {
		events = append(events, eventFunc(getContainerProgressName(ctr), reason))
	}
	return events
}

// ServiceConditionRunningOrHealthy is a service condition on status running or healthy
const ServiceConditionRunningOrHealthy = "running_or_healthy"

//nolint:gocyclo
func (s *composeService) waitDependencies(ctx context.Context, project *types.Project, dependant string, dependencies types.DependsOnConfig, containers Containers, timeout time.Duration) error {
	if timeout > 0 {
		withTimeout, cancelFunc := context.WithTimeout(ctx, timeout)
		defer cancelFunc()
		ctx = withTimeout
	}
	eg, ctx := errgroup.WithContext(ctx)
	for dep, config := range dependencies {
		if shouldWait, err := shouldWaitForDependency(dep, config, project); err != nil {
			return err
		} else if !shouldWait {
			continue
		}

		waitingFor := containers.filter(isService(dep), isNotOneOff)
		s.events.On(containerEvents(waitingFor, waiting)...)
		if len(waitingFor) == 0 {
			if config.Required {
				return fmt.Errorf("%s is missing dependency %s", dependant, dep)
			}
			logrus.Warnf("%s is missing dependency %s", dependant, dep)
			continue
		}

		eg.Go(func() error {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return nil
				}
				switch config.Condition {
				case ServiceConditionRunningOrHealthy:
					isHealthy, err := s.isServiceHealthy(ctx, waitingFor, true)
					if err != nil {
						if !config.Required {
							s.events.On(containerReasonEvents(waitingFor, skippedEvent,
								fmt.Sprintf("optional dependency %q is not running or is unhealthy", dep))...)
							logrus.Warnf("optional dependency %q is not running or is unhealthy: %s", dep, err.Error())
							return nil
						}
						return err
					}
					if isHealthy {
						s.events.On(containerEvents(waitingFor, healthy)...)
						return nil
					}
				case types.ServiceConditionHealthy:
					isHealthy, err := s.isServiceHealthy(ctx, waitingFor, false)
					if err != nil {
						if !config.Required {
							s.events.On(containerReasonEvents(waitingFor, skippedEvent,
								fmt.Sprintf("optional dependency %q failed to start", dep))...)
							logrus.Warnf("optional dependency %q failed to start: %s", dep, err.Error())
							return nil
						}
						s.events.On(containerEvents(waitingFor, func(s string) api.Resource {
							return errorEventf(s, "dependency %s failed to start", dep)
						})...)
						return fmt.Errorf("dependency failed to start: %w", err)
					}
					if isHealthy {
						s.events.On(containerEvents(waitingFor, healthy)...)
						return nil
					}
				case types.ServiceConditionCompletedSuccessfully:
					isExited, code, err := s.isServiceCompleted(ctx, waitingFor)
					if err != nil {
						return err
					}
					if isExited {
						if code == 0 {
							s.events.On(containerEvents(waitingFor, exited)...)
							return nil
						}

						messageSuffix := fmt.Sprintf("%q didn't complete successfully: exit %d", dep, code)
						if !config.Required {
							// optional -> mark as skipped & don't propagate error
							s.events.On(containerReasonEvents(waitingFor, skippedEvent,
								fmt.Sprintf("optional dependency %s", messageSuffix))...)
							logrus.Warnf("optional dependency %s", messageSuffix)
							return nil
						}

						msg := fmt.Sprintf("service %s", messageSuffix)
						s.events.On(containerEvents(waitingFor, func(s string) api.Resource {
							return errorEventf(s, "service %s", messageSuffix)
						})...)
						return errors.New(msg)
					}
				default:
					logrus.Warnf("unsupported depends_on condition: %s", config.Condition)
					return nil
				}
			}
		})
	}
	err := eg.Wait()
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("timeout waiting for dependencies")
	}
	return err
}

func shouldWaitForDependency(serviceName string, dependencyConfig types.ServiceDependency, project *types.Project) (bool, error) {
	if dependencyConfig.Condition == types.ServiceConditionStarted {
		// already managed by InDependencyOrder
		return false, nil
	}
	if service, err := project.GetService(serviceName); err != nil {
		for _, ds := range project.DisabledServices {
			if ds.Name == serviceName {
				// don't wait for disabled service (--no-deps)
				return false, nil
			}
		}
		return false, err
	} else if service.GetScale() == 0 {
		// don't wait for the dependency which configured to have 0 containers running
		return false, nil
	} else if service.Provider != nil {
		// don't wait for provider services
		return false, nil
	}
	return true, nil
}

func nextContainerNumber(containers []container.Summary) int {
	maxNumber := 0
	for _, c := range containers {
		s, ok := c.Labels[api.ContainerNumberLabel]
		if !ok {
			logrus.Warnf("container %s is missing %s label", c.ID, api.ContainerNumberLabel)
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			logrus.Warnf("container %s has invalid %s label: %s", c.ID, api.ContainerNumberLabel, s)
			continue
		}
		if n > maxNumber {
			maxNumber = n
		}
	}
	return maxNumber + 1
}

func (s *composeService) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name string, number int, opts createOptions,
) (ctr container.Summary, err error) {
	eventName := "Container " + name
	s.events.On(creatingEvent(eventName))
	ctr, err = s.createMobyContainer(ctx, project, service, name, number, nil, opts)
	if err != nil {
		if ctx.Err() == nil {
			s.events.On(api.Resource{
				ID:     eventName,
				Status: api.Error,
				Text:   err.Error(),
			})
		}
		return ctr, err
	}
	s.events.On(createdEvent(eventName))
	return ctr, nil
}

func (s *composeService) recreateContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	replaced container.Summary, inherit bool, timeout *time.Duration,
) (created container.Summary, err error) {
	eventName := getContainerProgressName(replaced)
	s.events.On(newEvent(eventName, api.Working, "Recreate"))
	defer func() {
		if err != nil && ctx.Err() == nil {
			s.events.On(api.Resource{
				ID:     eventName,
				Status: api.Error,
				Text:   err.Error(),
			})
		}
	}()

	number, err := strconv.Atoi(replaced.Labels[api.ContainerNumberLabel])
	if err != nil {
		return created, err
	}

	var inherited *container.Summary
	if inherit {
		inherited = &replaced
	}

	replacedContainerName := service.ContainerName
	if replacedContainerName == "" {
		replacedContainerName = service.Name + api.Separator + strconv.Itoa(number)
	}
	name := getContainerName(project.Name, service, number)
	tmpName := fmt.Sprintf("%s_%s", replaced.ID[:12], name)
	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            mergeLabels(service.Labels, service.CustomLabels).Add(api.ContainerReplaceLabel, replacedContainerName),
	}
	created, err = s.createMobyContainer(ctx, project, service, tmpName, number, inherited, opts)
	if err != nil {
		return created, err
	}

	timeoutInSecond := utils.DurationSecondToInt(timeout)
	_, err = s.apiClient().ContainerStop(ctx, replaced.ID, client.ContainerStopOptions{Timeout: timeoutInSecond})
	if err != nil {
		return created, err
	}

	_, err = s.apiClient().ContainerRemove(ctx, replaced.ID, client.ContainerRemoveOptions{})
	if err != nil {
		return created, err
	}

	_, err = s.apiClient().ContainerRename(ctx, tmpName, client.ContainerRenameOptions{
		NewName: name,
	})
	if err != nil {
		return created, err
	}

	s.events.On(newEvent(eventName, api.Done, "Recreated"))
	return created, err
}

// force sequential calls to ContainerStart to prevent race condition in engine assigning ports from ranges
var startMx sync.Mutex

func (s *composeService) startContainer(ctx context.Context, ctr container.Summary) error {
	s.events.On(newEvent(getContainerProgressName(ctr), api.Working, "Restart"))
	startMx.Lock()
	defer startMx.Unlock()
	_, err := s.apiClient().ContainerStart(ctx, ctr.ID, client.ContainerStartOptions{})
	if err != nil {
		return err
	}
	s.events.On(newEvent(getContainerProgressName(ctr), api.Done, "Restarted"))
	return nil
}

func (s *composeService) createMobyContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name string, number int, inherit *container.Summary, opts createOptions,
) (container.Summary, error) {
	var created container.Summary
	cfgs, err := s.getCreateConfigs(ctx, project, service, number, inherit, opts)
	if err != nil {
		return created, err
	}
	platform := service.Platform
	if platform == "" {
		platform = project.Environment["DOCKER_DEFAULT_PLATFORM"]
	}
	var plat *specs.Platform
	if platform != "" {
		var p specs.Platform
		p, err = platforms.Parse(platform)
		if err != nil {
			return created, err
		}
		plat = &p
	}

	response, err := s.apiClient().ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:             name,
		Platform:         plat,
		Config:           cfgs.Container,
		HostConfig:       cfgs.Host,
		NetworkingConfig: cfgs.Network,
	})
	if err != nil {
		return created, err
	}
	for _, warning := range response.Warnings {
		s.events.On(api.Resource{
			ID:     service.Name,
			Status: api.Warning,
			Text:   warning,
		})
	}
	res, err := s.apiClient().ContainerInspect(ctx, response.ID, client.ContainerInspectOptions{})
	if err != nil {
		return created, err
	}
	created = container.Summary{
		ID:     res.Container.ID,
		Labels: res.Container.Config.Labels,
		Names:  []string{res.Container.Name},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: res.Container.NetworkSettings.Networks,
		},
	}

	return created, nil
}

// getLinks mimics V1 compose/service.py::Service::_get_links()
func (s *composeService) getLinks(ctx context.Context, projectName string, service types.ServiceConfig, number int) ([]string, error) {
	var links []string
	format := func(k, v string) string {
		return fmt.Sprintf("%s:%s", k, v)
	}
	getServiceContainers := func(serviceName string) (Containers, error) {
		return s.getContainers(ctx, projectName, oneOffExclude, true, serviceName)
	}

	for _, rawLink := range service.Links {
		// linkName if informed like in: "serviceName[:linkName]"
		linkServiceName, linkName, ok := strings.Cut(rawLink, ":")
		if !ok {
			linkName = linkServiceName
		}
		cnts, err := getServiceContainers(linkServiceName)
		if err != nil {
			return nil, err
		}
		for _, c := range cnts {
			containerName := getCanonicalContainerName(c)
			links = append(links,
				format(containerName, linkName),
				format(containerName, linkServiceName+api.Separator+strconv.Itoa(number)),
				format(containerName, strings.Join([]string{projectName, linkServiceName, strconv.Itoa(number)}, api.Separator)),
			)
		}
	}

	if service.Labels[api.OneoffLabel] == "True" {
		cnts, err := getServiceContainers(service.Name)
		if err != nil {
			return nil, err
		}
		for _, c := range cnts {
			containerName := getCanonicalContainerName(c)
			links = append(links,
				format(containerName, service.Name),
				format(containerName, strings.TrimPrefix(containerName, projectName+api.Separator)),
				format(containerName, containerName),
			)
		}
	}

	for _, rawExtLink := range service.ExternalLinks {
		externalLink, linkName, ok := strings.Cut(rawExtLink, ":")
		if !ok {
			linkName = externalLink
		}
		links = append(links, format(externalLink, linkName))
	}
	return links, nil
}

func (s *composeService) isServiceHealthy(ctx context.Context, containers Containers, fallbackRunning bool) (bool, error) {
	for _, c := range containers {
		res, err := s.apiClient().ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		if err != nil {
			return false, err
		}
		ctr := res.Container
		name := ctr.Name[1:]

		if ctr.State.Status == container.StateExited {
			return false, fmt.Errorf("container %s exited (%d)", name, ctr.State.ExitCode)
		}

		noHealthcheck := ctr.Config.Healthcheck == nil || (len(ctr.Config.Healthcheck.Test) > 0 && ctr.Config.Healthcheck.Test[0] == "NONE")
		if noHealthcheck && fallbackRunning {
			// Container does not define a health check, but we can fall back to "running" state
			return ctr.State != nil && ctr.State.Status == container.StateRunning, nil
		}

		if ctr.State == nil || ctr.State.Health == nil {
			return false, fmt.Errorf("container %s has no healthcheck configured", name)
		}
		switch ctr.State.Health.Status {
		case container.Healthy:
			// Continue by checking the next container.
		case container.Unhealthy:
			return false, fmt.Errorf("container %s is unhealthy", name)
		case container.Starting:
			return false, nil
		default:
			return false, fmt.Errorf("container %s had unexpected health status %q", name, ctr.State.Health.Status)
		}
	}
	return true, nil
}

func (s *composeService) isServiceCompleted(ctx context.Context, containers Containers) (bool, int, error) {
	for _, c := range containers {
		res, err := s.apiClient().ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		if err != nil {
			return false, 0, err
		}
		if res.Container.State != nil && res.Container.State.Status == container.StateExited {
			return true, res.Container.State.ExitCode, nil
		}
	}
	return false, 0, nil
}

func (s *composeService) startService(ctx context.Context,
	project *types.Project, service types.ServiceConfig,
	containers Containers, listener api.ContainerEventListener,
	timeout time.Duration,
) error {
	if service.Deploy != nil && service.Deploy.Replicas != nil && *service.Deploy.Replicas == 0 {
		return nil
	}

	err := s.waitDependencies(ctx, project, service.Name, service.DependsOn, containers, timeout)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		if service.GetScale() == 0 {
			return nil
		}
		return fmt.Errorf("service %q has no container to start", service.Name)
	}

	for _, ctr := range containers.filter(isService(service.Name)) {
		if ctr.State == container.StateRunning {
			continue
		}

		err = s.injectSecrets(ctx, project, service, ctr.ID)
		if err != nil {
			return err
		}

		err = s.injectConfigs(ctx, project, service, ctr.ID)
		if err != nil {
			return err
		}

		eventName := getContainerProgressName(ctr)
		s.events.On(startingEvent(eventName))
		_, err = s.apiClient().ContainerStart(ctx, ctr.ID, client.ContainerStartOptions{})
		if err != nil {
			return err
		}

		for _, hook := range service.PostStart {
			err = s.runHook(ctx, ctr, service, hook, listener)
			if err != nil {
				return err
			}
		}

		s.events.On(startedEvent(eventName))
	}
	return nil
}

func mergeLabels(ls ...types.Labels) types.Labels {
	merged := types.Labels{}
	for _, l := range ls {
		maps.Copy(merged, l)
	}
	return merged
}
