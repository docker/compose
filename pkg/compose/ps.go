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
	"sort"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Ps(ctx context.Context, projectName string, options api.PsOptions) ([]api.ContainerSummary, error) {
	projectName = strings.ToLower(projectName)
	oneOff := oneOffExclude
	if options.All {
		oneOff = oneOffInclude
	}
	containers, err := s.getContainers(ctx, projectName, oneOff, options.All, options.Services...)
	if err != nil {
		return nil, err
	}

	if len(options.Services) != 0 {
		containers = containers.filter(isService(options.Services...))
	}
	summary := make([]api.ContainerSummary, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, ctr := range containers {
		eg.Go(func() error {
			var err error
			summary[i], err = s.containerSummary(ctx, ctr)
			return err
		})
	}
	return summary, eg.Wait()
}

// containerSummary builds the api summary for a container, inspecting it to
// retrieve its health and exit code
func (s *composeService) containerSummary(ctx context.Context, ctr container.Summary) (api.ContainerSummary, error) {
	inspect, err := s.apiClient().ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
	if err != nil {
		return api.ContainerSummary{}, err
	}
	health, exitCode := containerHealthAndExitCode(inspect)
	mounts, localVolumes := containerMounts(ctr)
	return api.ContainerSummary{
		ID:           ctr.ID,
		Name:         getCanonicalContainerName(ctr),
		Names:        ctr.Names,
		Image:        ctr.Image,
		Project:      ctr.Labels[api.ProjectLabel],
		Service:      ctr.Labels[api.ServiceLabel],
		Command:      ctr.Command,
		State:        ctr.State,
		Status:       ctr.Status,
		Created:      ctr.Created,
		Labels:       ctr.Labels,
		SizeRw:       ctr.SizeRw,
		SizeRootFs:   ctr.SizeRootFs,
		Mounts:       mounts,
		LocalVolumes: localVolumes,
		Networks:     containerNetworks(ctr),
		Health:       health,
		ExitCode:     exitCode,
		Publishers:   containerPublishers(ctr),
	}, nil
}

func containerPublishers(ctr container.Summary) []api.PortPublisher {
	sort.Slice(ctr.Ports, func(i, j int) bool {
		return ctr.Ports[i].PrivatePort < ctr.Ports[j].PrivatePort
	})
	publishers := make([]api.PortPublisher, len(ctr.Ports))
	for i, p := range ctr.Ports {
		var url string
		if p.IP.IsValid() {
			url = p.IP.String()
		}
		publishers[i] = api.PortPublisher{
			URL:           url, // TODO(thaJeztah); change this to a netip.Addr ??
			TargetPort:    int(p.PrivatePort),
			PublishedPort: int(p.PublicPort),
			Protocol:      p.Type,
		}
	}
	return publishers
}

func containerHealthAndExitCode(inspect client.ContainerInspectResult) (container.HealthStatus, int) {
	var (
		health   container.HealthStatus
		exitCode int
	)
	state := inspect.Container.State
	if state == nil {
		return health, exitCode
	}
	switch state.Status {
	case container.StateRunning:
		if state.Health != nil {
			health = state.Health.Status
		}
	case container.StateExited, container.StateDead:
		exitCode = state.ExitCode
	}
	return health, exitCode
}

func containerMounts(ctr container.Summary) ([]string, int) {
	var (
		local  int
		mounts []string
	)
	for _, m := range ctr.Mounts {
		name := m.Name
		if name == "" {
			name = m.Source
		}
		if m.Driver == "local" {
			local++
		}
		mounts = append(mounts, name)
	}
	return mounts, local
}

func containerNetworks(ctr container.Summary) []string {
	var networks []string
	if ctr.NetworkSettings != nil {
		for k := range ctr.NetworkSettings.Networks {
			networks = append(networks, k)
		}
	}
	return networks
}
