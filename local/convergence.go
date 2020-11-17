// +build local

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

package local

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/network"

	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/progress"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *local) ensureService(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	actual, err := s.containerService.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+project.Name),
			filters.Arg("label", "com.docker.compose.service="+service.Name),
		),
	})
	if err != nil {
		return err
	}

	expected, err := jsonHash(service)
	if err != nil {
		return err
	}

	if len(actual) == 0 {
		return s.createContainer(ctx, project, service)
	}

	container := actual[0] // TODO handle services with replicas
	diverged := container.Labels["com.docker.compose.config-hash"] != expected
	if diverged {
		return s.recreateContainer(ctx, project, service, container)
	}

	if container.State == "running" {
		// already running, skip
		return nil
	}

	return s.restartContainer(ctx, service, container)
}

func (s *local) createContainer(ctx context.Context, project *types.Project, service types.ServiceConfig) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Working,
		StatusText: "Create",
		Done:       false,
	})
	name := fmt.Sprintf("%s_%s", project.Name, service.Name)
	err := s.runContainer(ctx, project, service, name, nil)
	if err != nil {
		return err
	}
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Done,
		StatusText: "Created",
		Done:       true,
	})
	return nil
}

func (s *local) recreateContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, container moby.Container) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Working,
		StatusText: "Recreate",
		Done:       false,
	})
	err := s.containerService.Stop(ctx, container.ID, nil)
	if err != nil {
		return err
	}
	name := getContainerName(container)
	tmpName := fmt.Sprintf("%s_%s", container.ID[:12], name)
	err = s.containerService.apiClient.ContainerRename(ctx, container.ID, tmpName)
	if err != nil {
		return err
	}
	err = s.runContainer(ctx, project, service, name, &container)
	if err != nil {
		return err
	}
	err = s.containerService.Delete(ctx, container.ID, containers.DeleteRequest{})
	if err != nil {
		return err
	}
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Done,
		StatusText: "Recreated",
		Done:       true,
	})
	return nil
}

func (s *local) restartContainer(ctx context.Context, service types.ServiceConfig, container moby.Container) error {
	w := progress.ContextWriter(ctx)
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Working,
		StatusText: "Restart",
		Done:       false,
	})
	err := s.containerService.Start(ctx, container.ID)
	if err != nil {
		return err
	}
	w.Event(progress.Event{
		ID:         fmt.Sprintf("Service %q", service.Name),
		Status:     progress.Done,
		StatusText: "Restarted",
		Done:       true,
	})
	return nil
}

func (s *local) runContainer(ctx context.Context, project *types.Project, service types.ServiceConfig, name string, container *moby.Container) error {
	containerConfig, hostConfig, networkingConfig, err := getContainerCreateOptions(project, service, container)
	if err != nil {
		return err
	}
	id, err := s.containerService.create(ctx, containerConfig, hostConfig, networkingConfig, name)
	if err != nil {
		return err
	}
	for net := range service.Networks {
		name := fmt.Sprintf("%s_%s", project.Name, net)
		err = s.connectContainerToNetwork(ctx, id, service.Name, name)
		if err != nil {
			return err
		}
	}
	err = s.containerService.apiClient.ContainerStart(ctx, id, moby.ContainerStartOptions{})
	if err != nil {
		return err
	}
	return nil
}


func (s *local) connectContainerToNetwork(ctx context.Context, id string, service string, n string) error {
	err := s.containerService.apiClient.NetworkConnect(ctx, n, id, &network.EndpointSettings{
		Aliases: []string{service},
	})
	if err != nil {
		return err
	}
	return nil
}
