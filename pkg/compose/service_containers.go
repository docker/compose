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
	"maps"
	"strconv"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/platforms"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/api"
)

// This file gathers the per-service container helpers shared by the
// plan-based engine (reconcile.go and the executor, entered through
// create/up/start) and by the commands that still walk the dependency order
// themselves (internal/depgraph: restart, stop, down and the classic build). It
// covers container naming, resolution of service references (volumes_from,
// network_mode/ipc/pid, links) and container creation through the Docker API;
// the waits on depends_on conditions are in dependency_wait.go.

const (
	doubledContainerNameWarning = "WARNING: The %q service is using the custom container name %q. " +
		"Docker requires each container to have a unique name. " +
		"Remove the custom name to scale the service"
)

// getScale returns the number of replicas the service must run. A service
// pinned to a custom container_name cannot scale beyond one replica, as every
// container needs a distinct name: this is rejected here rather than at
// container-creation time.
func getScale(config types.ServiceConfig) (int, error) {
	scale := config.GetScale()
	if scale > 1 && config.ContainerName != "" {
		return 0, fmt.Errorf(doubledContainerNameWarning,
			config.Name,
			config.ContainerName)
	}
	return scale, nil
}

// resolveServiceReferences replaces references to other services with references
// to actual container IDs. It resolves VolumesFrom, NetworkMode, IPC and PID
// shared namespaces. The containersByService map provides the observed containers
// grouped by service name.
func resolveServiceReferences(service *types.ServiceConfig, containersByService map[string]Containers) error {
	if err := resolveVolumeFrom(service, containersByService); err != nil {
		return err
	}
	return resolveSharedNamespaces(service, containersByService)
}

func resolveVolumeFrom(service *types.ServiceConfig, containersByService map[string]Containers) error {
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
		dependencies := containersByService[name]
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share volume with service %s: container missing", name)
		}
		service.VolumesFrom[i] = dependencies.sorted()[0].ID
	}
	return nil
}

func resolveSharedNamespaces(service *types.ServiceConfig, containersByService map[string]Containers) error {
	if name := getDependentServiceFromMode(service.NetworkMode); name != "" {
		dependencies := containersByService[name]
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share network namespace with service %s: container missing", name)
		}
		service.NetworkMode = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	if name := getDependentServiceFromMode(service.Ipc); name != "" {
		dependencies := containersByService[name]
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share IPC namespace with service %s: container missing", name)
		}
		service.Ipc = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	if name := getDependentServiceFromMode(service.Pid); name != "" {
		dependencies := containersByService[name]
		if len(dependencies) == 0 {
			return fmt.Errorf("cannot share PID namespace with service %s: container missing", name)
		}
		service.Pid = types.ContainerPrefix + dependencies.sorted()[0].ID
	}

	return nil
}

func getContainerProgressName(ctr container.Summary) string {
	return "Container " + naming.CanonicalContainerName(ctr)
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

// errNoContainerToStart is the failure of a service the start phase finds no
// container to start for (see planServiceStart).
func errNoContainerToStart(service string) error {
	return fmt.Errorf("service %q has no container to start", service)
}

func nextContainerNumber(containers []container.Summary) int {
	maxNumber := 0
	for _, ctr := range containers {
		s, ok := ctr.Labels[api.ContainerNumberLabel]
		if !ok {
			logrus.Warnf("container %s is missing %s label", ctr.ID, api.ContainerNumberLabel)
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			logrus.Warnf("container %s has invalid %s label: %s", ctr.ID, api.ContainerNumberLabel, s)
			continue
		}
		if n > maxNumber {
			maxNumber = n
		}
	}
	return maxNumber + 1
}

func (s *composeService) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name string, number int, options createOptions,
) (ctr container.Summary, err error) {
	eventName := "Container " + name
	s.events.On(progress.Creating(eventName))
	ctr, err = s.createMobyContainer(ctx, project, service, name, number, nil, options)
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
	s.events.On(progress.Created(eventName))
	return ctr, nil
}

// startMx serializes the plan executor's ContainerStart calls across the whole
// process. It was added by docker/compose#12851 as a workaround for
// docker/compose#12846, which was not a race between container starts but a
// flaw of the daemon's port allocator: with Engine 28.0 to 28.2, a container
// with several random published ports, one on 0.0.0.0 and one on a specific
// address, could be given the same host port twice. That flaw is fixed since
// Engine 28.3.0 (moby/moby#50054). A lock around ContainerStart only orders
// the starts, so it was unlikely to help against that flaw, and no other
// reason to keep the mutex is known.
//
// It is not what starts the replicas of a service one after the other, which
// docker/compose#8530 (a port range shared by the replicas) required and
// docker/compose#10067 introduced: that order is the plan's replica chain
// (each replica's start waits for the previous one's), and does not depend on
// this mutex.
//
// Only the plan executor takes it (a relay restarted by a start plan
// included): restart, run, the deployment of the relay and the hook runners
// start their containers without it.
//
// TODO: remove startMx, and the locking around it, once Engine 28.x is out of
// support.
//
// While it exists, it deliberately stays a package-level variable rather than
// a composeService field: what it serializes is a call to the daemon, not
// something of a service instance, so two instances (e.g. SDK users) driving
// the same daemon must still exclude each other. A per-instance lock would
// silently stop serializing the starts of two instances.
var startMx sync.Mutex

func (s *composeService) createMobyContainer(ctx context.Context, project *types.Project, service types.ServiceConfig,
	name string, number int, inherit *container.Summary, options createOptions,
) (container.Summary, error) {
	var created container.Summary
	cfgs, err := s.getCreateConfigs(ctx, project, service, number, inherit, options)
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
	// Starting API version 1.44, the ContainerCreate API call takes multiple networks
	// so we include all configurations there and can skip the one-by-one calls here.
	// For older API versions (e.g. Docker 20.10/API 1.41, Synology DSM 7.1/7.2),
	// extra networks must be connected individually after creation via NetworkConnect.
	apiVersion, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return created, err
	}
	if versions.LessThan(apiVersion, apiVersion144) {
		serviceNetworks := service.NetworksByPriority()
		for _, networkKey := range serviceNetworks {
			mobyNetworkName := project.Networks[networkKey].Name
			if string(cfgs.Host.NetworkMode) == mobyNetworkName {
				// primary network already configured as part of ContainerCreate
				continue
			}
			epSettings, err := createEndpointSettings(project, service, number, networkKey, cfgs.Links, options.UseNetworkAliases)
			if err != nil {
				_, _ = s.apiClient().ContainerRemove(ctx, response.ID, client.ContainerRemoveOptions{Force: true})
				return created, err
			}
			if _, err := s.apiClient().NetworkConnect(ctx, mobyNetworkName, client.NetworkConnectOptions{
				Container:      response.ID,
				EndpointConfig: epSettings,
			}); err != nil {
				_, _ = s.apiClient().ContainerRemove(ctx, response.ID, client.ContainerRemoveOptions{Force: true})
				return created, err
			}
		}
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
		serviceContainers, err := getServiceContainers(linkServiceName)
		if err != nil {
			return nil, err
		}
		for _, ctr := range serviceContainers {
			containerName := naming.CanonicalContainerName(ctr)
			links = append(links,
				format(containerName, linkName),
				format(containerName, linkServiceName+api.Separator+strconv.Itoa(number)),
				format(containerName, strings.Join([]string{projectName, linkServiceName, strconv.Itoa(number)}, api.Separator)),
			)
		}
	}

	if service.Labels[api.OneoffLabel] == "True" {
		serviceContainers, err := getServiceContainers(service.Name)
		if err != nil {
			return nil, err
		}
		for _, ctr := range serviceContainers {
			containerName := naming.CanonicalContainerName(ctr)
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

func mergeLabels(ls ...types.Labels) types.Labels {
	merged := types.Labels{}
	for _, l := range ls {
		maps.Copy(merged, l)
	}
	return merged
}
