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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/platforms"
	"github.com/docker/compose/v2/internal/tracing"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/versions"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

const (
	extLifecycle  = "x-lifecycle"
	forceRecreate = "force_recreate"

	doubledContainerNameWarning = "WARNING: The %q service is using the custom container name %q. " +
		"Docker requires each container to have a unique name. " +
		"Remove the custom name to scale the service.\n"
)

// convergence manages service's container lifecycle.
// Based on initially observed state, it reconciles the existing container with desired state, which might include
// re-creating container, adding or removing replicas, or starting stopped containers.
// Cross services dependencies are managed by creating services in expected order and updating `service:xx` reference
// when a service has converged, so dependent ones can be managed with resolved containers references.
type convergence struct {
	service       *composeService
	observedState map[string]Containers
	stateMutex    sync.Mutex
}

func (c *convergence) getObservedState(serviceName string) Containers {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	return c.observedState[serviceName]
}

func (c *convergence) setObservedState(serviceName string, containers Containers) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	c.observedState[serviceName] = containers
}

func newConvergence(services []string, state Containers, s *composeService) *convergence {
	observedState := map[string]Containers{}
	for _, s := range services {
		observedState[s] = Containers{}
	}
	for _, c := range state.filter(isNotOneOff) {
		service := c.Labels[api.ServiceLabel]
		observedState[service] = append(observedState[service], c)
	}
	return &convergence{
		service:       s,
		observedState: observedState,
	}
}

func (c *convergence) apply(ctx context.Context, project *types.Project, options api.CreateOptions) error {
	return InDependencyOrder(ctx, project, func(ctx context.Context, name string) error {
		service, err := project.GetService(name)
		if err != nil {
			return err
		}

		return tracing.SpanWrapFunc("service/apply", tracing.ServiceOptions(service), func(ctx context.Context) error {
			strategy := options.RecreateDependencies
			if utils.StringContains(options.Services, name) {
				strategy = options.Recreate
			}
			return c.ensureService(ctx, project, service, strategy, options.Inherit, options.Timeout)
		})(ctx)
	})
}

var mu sync.Mutex

func (c *convergence) ensureService(ctx context.Context, project *types.Project, service types.ServiceConfig, recreate string, inherit bool, timeout *time.Duration) error {
	expected, err := getScale(service)
	if err != nil {
		return err
	}
	containers := c.getObservedState(service.Name)
	actual := len(containers)
	updated := make(Containers, expected)

	eg, _ := errgroup.WithContext(ctx)

	err = c.resolveServiceReferences(&service)
	if err != nil {
		return err
	}

	sort.Slice(containers, func(i, j int) bool {
		// select obsolete containers first, so they get removed as we scale down
		if obsolete, _ := mustRecreate(service, containers[i], recreate); obsolete {
			// i is obsolete, so must be first in the list
			return true
		}
		if obsolete, _ := mustRecreate(service, containers[j], recreate); obsolete {
			// j is obsolete, so must be first in the list
			return false
		}

		// For up-to-date containers, sort by container number to preserve low-values in container numbers
		ni, erri := strconv.Atoi(containers[i].Labels[api.ContainerNumberLabel])
		nj, errj := strconv.Atoi(containers[j].Labels[api.ContainerNumberLabel])
		if erri == nil && errj == nil {
			return ni < nj
		}

		// If we don't get a container number (?) just sort by creation date
		return containers[i].Created < containers[j].Created
	})
	for i, container := range containers {
		if i >= expected {
			// Scale Down
			container := container
			traceOpts := append(tracing.ServiceOptions(service), tracing.ContainerOptions(container)...)
			eg.Go(tracing.SpanWrapFuncForErrGroup(ctx, "service/scale/down", traceOpts, func(ctx context.Context) error {
				return c.service.stopAndRemoveContainer(ctx, container, timeout, false)
			}))
			continue
		}

		mustRecreate, err := mustRecreate(service, container, recreate)
		if err != nil {
			return err
		}
		if mustRecreate {
			i, container := i, container
			eg.Go(tracing.SpanWrapFuncForErrGroup(ctx, "container/recreate", tracing.ContainerOptions(container), func(ctx context.Context) error {
				recreated, err := c.service.recreateContainer(ctx, project, service, container, inherit, timeout)
				updated[i] = recreated
				return err
			}))
			continue
		}

		// Enforce non-diverged containers are running
		w := progress.ContextWriter(ctx)
		name := getContainerProgressName(container)
		switch container.State {
		case ContainerRunning:
			w.Event(progress.RunningEvent(name))
		case ContainerCreated:
		case ContainerRestarting:
		case ContainerExited:
			w.Event(progress.CreatedEvent(name))
		default:
			container := container
			eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "service/start", tracing.ContainerOptions(container), func(ctx context.Context) error {
				return c.service.startContainer(ctx, container)
			}))
		}
		updated[i] = container
	}

	next := nextContainerNumber(containers)
	for i := 0; i < expected-actual; i++ {
		// Scale UP
		number := next + i
		name := getContainerName(project.Name, service, number)
		i := i
		eventOpts := tracing.SpanOptions{trace.WithAttributes(attribute.String("container.name", name))}
		eg.Go(tracing.EventWrapFuncForErrGroup(ctx, "service/scale/up", eventOpts, func(ctx context.Context) error {
			opts := createOptions{
				AutoRemove:        false,
				AttachStdin:       false,
				UseNetworkAliases: true,
				Labels:            mergeLabels(service.Labels, service.CustomLabels),
			}
			container, err := c.service.createContainer(ctx, project, service, name, number, opts)
			updated[actual+i] = container
			return err
		}))
		continue
	}

	err = eg.Wait()
	c.setObservedState(service.Name, updated)
	return err
}

func getScale(config types.ServiceConfig) (int, error) {
	scale := config.GetScale()
	if scale > 1 && config.ContainerName != "" {
		return 0, fmt.Errorf(doubledContainerNameWarning,
			config.Name,
			config.ContainerName)
	}
	return scale, nil
}

// resolveServiceReferences replaces reference to another service with reference to an actual container
func (c *convergence) resolveServiceReferences(service *types.ServiceConfig) error {
	err := c.resolveVolumeFrom(service)
	if err != nil {
		return err
	}

	err = c.resolveSharedNamespaces(service)
	if err != nil {
		return err
	}
	return nil
}

func (c *convergence) resolveVolumeFrom(service *types.ServiceConfig) error {
	for i, vol := range service.VolumesFrom {
		spec := strings.Split(vol, ":")
		if len(spec) == 0 {
			continue
		}
		if spec[0] == "container" {
			service.VolumesFrom[i] = spec[1]
			continue
		}
		name := spec[0]
		dependencies := c.getObservedState(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share volume with service %s: container missing", name)
		}
		service.VolumesFrom[i] = dependencies.sorted()[0].ID
	}
	return nil
}

func (c *convergence) resolveSharedNamespaces(service *types.ServiceConfig) error {
	str := service.NetworkMode
	if name := getDependentServiceFromMode(str); name != "" {
		dependencies := c.getObservedState(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share network namespace with service %s: container missing", name)
		}
		service.NetworkMode = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	str = service.Ipc
	if name := getDependentServiceFromMode(str); name != "" {
		dependencies := c.getObservedState(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share IPC namespace with service %s: container missing", name)
		}
		service.Ipc = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	str = service.Pid
	if name := getDependentServiceFromMode(str); name != "" {
		dependencies := c.getObservedState(name)
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share PID namespace with service %s: container missing", name)
		}
		service.Pid = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	return nil
}

func mustRecreate(expected types.ServiceConfig, actual moby.Container, policy string) (bool, error) {
	if policy == api.RecreateNever {
		return false, nil
	}
	if policy == api.RecreateForce || expected.Extensions[extLifecycle] == forceRecreate {
		return true, nil
	}
	configHash, err := ServiceHash(expected)
	if err != nil {
		return false, err
	}
	configChanged := actual.Labels[api.ConfigHashLabel] != configHash
	imageUpdated := actual.Labels[api.ImageDigestLabel] != expected.CustomLabels[api.ImageDigestLabel]
	return configChanged || imageUpdated, nil
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

func getContainerProgressName(container moby.Container) string {
	return "Container " + getCanonicalContainerName(container)
}

func containerEvents(containers Containers, eventFunc func(string) progress.Event) []progress.Event {
	events := []progress.Event{}
	for _, container := range containers {
		events = append(events, eventFunc(getContainerProgressName(container)))
	}
	return events
}

func containerReasonEvents(containers Containers, eventFunc func(string, string) progress.Event, reason string) []progress.Event {
	events := []progress.Event{}
	for _, container := range containers {
		events = append(events, eventFunc(getContainerProgressName(container), reason))
	}
	return events
}

// ServiceConditionRunningOrHealthy is a service condition on status running or healthy
const ServiceConditionRunningOrHealthy = "running_or_healthy"

//nolint:gocyclo
func (s *composeService) waitDependencies(ctx context.Context, project *types.Project, dependant string, dependencies types.DependsOnConfig, containers Containers) error {
	eg, _ := errgroup.WithContext(ctx)
	w := progress.ContextWriter(ctx)
	for dep, config := range dependencies {
		if shouldWait, err := shouldWaitForDependency(dep, config, project); err != nil {
			return err
		} else if !shouldWait {
			continue
		}

		waitingFor := containers.filter(isService(dep))
		w.Events(containerEvents(waitingFor, progress.Waiting))
		if len(waitingFor) == 0 {
			if config.Required {
				return fmt.Errorf("%s is missing dependency %s", dependant, dep)
			}
			logrus.Warnf("%s is missing dependency %s", dependant, dep)
			continue
		}

		dep, config := dep, config
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
					healthy, err := s.isServiceHealthy(ctx, waitingFor, true)
					if err != nil {
						if !config.Required {
							w.Events(containerReasonEvents(waitingFor, progress.SkippedEvent, fmt.Sprintf("optional dependency %q is not running or is unhealthy", dep)))
							logrus.Warnf("optional dependency %q is not running or is unhealthy: %s", dep, err.Error())
							return nil
						}
						return err
					}
					if healthy {
						w.Events(containerEvents(waitingFor, progress.Healthy))
						return nil
					}
				case types.ServiceConditionHealthy:
					healthy, err := s.isServiceHealthy(ctx, waitingFor, false)
					if err != nil {
						if !config.Required {
							w.Events(containerReasonEvents(waitingFor, progress.SkippedEvent, fmt.Sprintf("optional dependency %q failed to start", dep)))
							logrus.Warnf("optional dependency %q failed to start: %s", dep, err.Error())
							return nil
						}
						w.Events(containerEvents(waitingFor, progress.ErrorEvent))
						return fmt.Errorf("dependency failed to start: %w", err)
					}
					if healthy {
						w.Events(containerEvents(waitingFor, progress.Healthy))
						return nil
					}
				case types.ServiceConditionCompletedSuccessfully:
					exited, code, err := s.isServiceCompleted(ctx, waitingFor)
					if err != nil {
						return err
					}
					if exited {
						if code == 0 {
							w.Events(containerEvents(waitingFor, progress.Exited))
							return nil
						}

						messageSuffix := fmt.Sprintf("%q didn't complete successfully: exit %d", dep, code)
						if !config.Required {
							// optional -> mark as skipped & don't propagate error
							w.Events(containerReasonEvents(waitingFor, progress.SkippedEvent, fmt.Sprintf("optional dependency %s", messageSuffix)))
							logrus.Warnf("optional dependency %s", messageSuffix)
							return nil
						}

						msg := fmt.Sprintf("service %s", messageSuffix)
						w.Events(containerReasonEvents(waitingFor, progress.ErrorMessageEvent, msg))
						return errors.New(msg)
					}
				default:
					logrus.Warnf("unsupported depends_on condition: %s", config.Condition)
					return nil
				}
			}
		})
	}
	return eg.Wait()
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
	}
	return true, nil
}

func nextContainerNumber(containers []moby.Container) int {
	max := 0
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
		if n > max {
			max = n
		}
	}
	return max + 1

}

func (s *composeService) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name string, number int, opts createOptions) (container moby.Container, err error) {
	w := progress.ContextWriter(ctx)
	eventName := "Container " + name
	w.Event(progress.CreatingEvent(eventName))
	container, err = s.createMobyContainer(ctx, project, service, name, number, nil, opts, w)
	if err != nil {
		return
	}
	w.Event(progress.CreatedEvent(eventName))
	return
}

func (s *composeService) recreateContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	replaced moby.Container, inherit bool, timeout *time.Duration) (moby.Container, error) {
	var created moby.Container
	w := progress.ContextWriter(ctx)
	w.Event(progress.NewEvent(getContainerProgressName(replaced), progress.Working, "Recreate"))

	number, err := strconv.Atoi(replaced.Labels[api.ContainerNumberLabel])
	if err != nil {
		return created, err
	}

	var inherited *moby.Container
	if inherit {
		inherited = &replaced
	}
	name := getContainerName(project.Name, service, number)
	tmpName := fmt.Sprintf("%s_%s", replaced.ID[:12], name)
	opts := createOptions{
		AutoRemove:        false,
		AttachStdin:       false,
		UseNetworkAliases: true,
		Labels:            mergeLabels(service.Labels, service.CustomLabels).Add(api.ContainerReplaceLabel, replaced.ID),
	}
	created, err = s.createMobyContainer(ctx, project, service, tmpName, number, inherited, opts, w)
	if err != nil {
		return created, err
	}

	timeoutInSecond := utils.DurationSecondToInt(timeout)
	err = s.apiClient().ContainerStop(ctx, replaced.ID, containerType.StopOptions{Timeout: timeoutInSecond})
	if err != nil {
		return created, err
	}

	err = s.apiClient().ContainerRemove(ctx, replaced.ID, containerType.RemoveOptions{})
	if err != nil {
		return created, err
	}

	err = s.apiClient().ContainerRename(ctx, created.ID, name)
	if err != nil {
		return created, err
	}

	w.Event(progress.NewEvent(getContainerProgressName(replaced), progress.Done, "Recreated"))
	setDependentLifecycle(project, service.Name, forceRecreate)
	return created, err
}

// setDependentLifecycle define the Lifecycle strategy for all services to depend on specified service
func setDependentLifecycle(project *types.Project, service string, strategy string) {
	mu.Lock()
	defer mu.Unlock()

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
	err := s.apiClient().ContainerStart(ctx, container.ID, containerType.StartOptions{})
	if err != nil {
		return err
	}
	w.Event(progress.NewEvent(getContainerProgressName(container), progress.Done, "Restarted"))
	return nil
}

func (s *composeService) createMobyContainer(ctx context.Context,
	project *types.Project,
	service types.ServiceConfig,
	name string,
	number int,
	inherit *moby.Container,
	opts createOptions,
	w progress.Writer,
) (moby.Container, error) {
	var created moby.Container
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

	response, err := s.apiClient().ContainerCreate(ctx, cfgs.Container, cfgs.Host, cfgs.Network, plat, name)
	if err != nil {
		return created, err
	}
	for _, warning := range response.Warnings {
		w.Event(progress.Event{
			ID:     service.Name,
			Status: progress.Warning,
			Text:   warning,
		})
	}
	inspectedContainer, err := s.apiClient().ContainerInspect(ctx, response.ID)
	if err != nil {
		return created, err
	}
	created = moby.Container{
		ID:     inspectedContainer.ID,
		Labels: inspectedContainer.Config.Labels,
		Names:  []string{inspectedContainer.Name},
		NetworkSettings: &moby.SummaryNetworkSettings{
			Networks: inspectedContainer.NetworkSettings.Networks,
		},
	}

	apiVersion, err := s.RuntimeVersion(ctx)
	if err != nil {
		return created, err
	}
	// Starting API version 1.44, the ContainerCreate API call takes multiple networks
	// so we include all the configurations there and can skip the one-by-one calls here
	if versions.LessThan(apiVersion, "1.44") {
		// the highest-priority network is the primary and is included in the ContainerCreate API
		// call via container.NetworkMode & network.NetworkingConfig
		// any remaining networks are connected one-by-one here after creation (but before start)
		serviceNetworks := service.NetworksByPriority()
		for _, networkKey := range serviceNetworks {
			mobyNetworkName := project.Networks[networkKey].Name
			if string(cfgs.Host.NetworkMode) == mobyNetworkName {
				// primary network already configured as part of ContainerCreate
				continue
			}
			epSettings := createEndpointSettings(project, service, number, networkKey, cfgs.Links, opts.UseNetworkAliases)
			if err := s.apiClient().NetworkConnect(ctx, mobyNetworkName, created.ID, epSettings); err != nil {
				return created, err
			}
		}
	}

	err = s.injectSecrets(ctx, project, service, created.ID)
	if err != nil {
		return created, err
	}

	err = s.injectConfigs(ctx, project, service, created.ID)
	return created, err
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
		linkSplit := strings.Split(rawLink, ":")
		linkServiceName := linkSplit[0]
		linkName := linkServiceName
		if len(linkSplit) == 2 {
			linkName = linkSplit[1] // linkName if informed like in: "serviceName:linkName"
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
		extLinkSplit := strings.Split(rawExtLink, ":")
		externalLink := extLinkSplit[0]
		linkName := externalLink
		if len(extLinkSplit) == 2 {
			linkName = extLinkSplit[1]
		}
		links = append(links, format(externalLink, linkName))
	}
	return links, nil
}

func (s *composeService) isServiceHealthy(ctx context.Context, containers Containers, fallbackRunning bool) (bool, error) {
	for _, c := range containers {
		container, err := s.apiClient().ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, err
		}
		name := container.Name[1:]

		if container.State.Status == "exited" {
			return false, fmt.Errorf("container %s exited (%d)", name, container.State.ExitCode)
		}

		if container.Config.Healthcheck == nil && fallbackRunning {
			// Container does not define a health check, but we can fall back to "running" state
			return container.State != nil && container.State.Status == "running", nil
		}

		if container.State == nil || container.State.Health == nil {
			return false, fmt.Errorf("container %s has no healthcheck configured", name)
		}
		switch container.State.Health.Status {
		case moby.Healthy:
			// Continue by checking the next container.
		case moby.Unhealthy:
			return false, fmt.Errorf("container %s is unhealthy", name)
		case moby.Starting:
			return false, nil
		default:
			return false, fmt.Errorf("container %s had unexpected health status %q", name, container.State.Health.Status)
		}
	}
	return true, nil
}

func (s *composeService) isServiceCompleted(ctx context.Context, containers Containers) (bool, int, error) {
	for _, c := range containers {
		container, err := s.apiClient().ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, 0, err
		}
		if container.State != nil && container.State.Status == "exited" {
			return true, container.State.ExitCode, nil
		}
	}
	return false, 0, nil
}

func (s *composeService) startService(ctx context.Context, project *types.Project, service types.ServiceConfig, containers Containers) error {
	if service.Deploy != nil && service.Deploy.Replicas != nil && *service.Deploy.Replicas == 0 {
		return nil
	}

	err := s.waitDependencies(ctx, project, service.Name, service.DependsOn, containers)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		if service.GetScale() == 0 {
			return nil
		}
		return fmt.Errorf("service %q has no container to start", service.Name)
	}

	w := progress.ContextWriter(ctx)
	for _, container := range containers.filter(isService(service.Name)) {
		if container.State == ContainerRunning {
			continue
		}
		eventName := getContainerProgressName(container)
		w.Event(progress.StartingEvent(eventName))
		err := s.apiClient().ContainerStart(ctx, container.ID, containerType.StartOptions{})
		if err != nil {
			return err
		}
		w.Event(progress.StartedEvent(eventName))
	}
	return nil
}

func mergeLabels(ls ...types.Labels) types.Labels {
	merged := types.Labels{}
	for _, l := range ls {
		for k, v := range l {
			merged[k] = v
		}
	}
	return merged
}
