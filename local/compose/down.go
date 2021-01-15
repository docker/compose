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
	"path/filepath"
	"strings"

	"github.com/docker/compose-cli/api/compose"

	"github.com/docker/compose-cli/api/progress"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	eg, _ := errgroup.WithContext(ctx)
	w := progress.ContextWriter(ctx)

	project, err := s.projectFromContainerLabels(ctx, projectName)
	if err != nil {
		return err
	}

	var containers Containers
	containers, err = s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(project.Name)),
		All:     true,
	})
	if err != nil {
		return err
	}

	err = InReverseDependencyOrder(ctx, project, func(c context.Context, service types.ServiceConfig) error {
		serviceContainers, others := containers.split(isService(service.Name))
		s.removeContainers(ctx, w, eg, serviceContainers)
		containers = others
		return err
	})

	if options.RemoveOrphans {
		s.removeContainers(ctx, w, eg, containers)
		if err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}
	err = eg.Wait()
	if err != nil {
		return err
	}
	networks, err := s.apiClient.NetworkList(ctx, moby.NetworkListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
		),
	})
	if err != nil {
		return err
	}
	for _, n := range networks {
		networkID := n.ID
		networkName := n.Name
		eg.Go(func() error {
			return s.ensureNetworkDown(ctx, networkID, networkName)
		})
	}

	return eg.Wait()
}

func (s *composeService) removeContainers(ctx context.Context, w progress.Writer, eg *errgroup.Group, containers []moby.Container) {
	for _, container := range containers {
		toDelete := container
		eg.Go(func() error {
			eventName := "Container " + getCanonicalContainerName(toDelete)
			w.Event(progress.StoppingEvent(eventName))
			err := s.apiClient.ContainerStop(ctx, toDelete.ID, nil)
			if err != nil {
				w.Event(progress.ErrorMessageEvent(eventName, "Error while Stopping"))
				return err
			}
			w.Event(progress.RemovingEvent(eventName))
			err = s.apiClient.ContainerRemove(ctx, toDelete.ID, moby.ContainerRemoveOptions{Force: true})
			if err != nil {
				w.Event(progress.ErrorMessageEvent(eventName, "Error while Removing"))
				return err
			}
			w.Event(progress.RemovedEvent(eventName))
			return nil
		})
	}
}

func (s *composeService) projectFromContainerLabels(ctx context.Context, projectName string) (*types.Project, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
		),
		All: true,
	})
	if err != nil {
		return nil, err
	}
	fakeProject := &types.Project{
		Name: projectName,
	}
	if len(containers) == 0 {
		return fakeProject, nil
	}
	options, err := loadProjectOptionsFromLabels(containers[0])
	if err != nil {
		return nil, err
	}
	if options.ConfigPaths[0] == "-" {
		for _, container := range containers {
			fakeProject.Services = append(fakeProject.Services, types.ServiceConfig{
				Name: container.Labels[serviceLabel],
			})
		}
		return fakeProject, nil
	}
	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return nil, err
	}

	return project, nil
}

func loadProjectOptionsFromLabels(c moby.Container) (*cli.ProjectOptions, error) {
	var configFiles []string
	relativePathConfigFiles := strings.Split(c.Labels[configFilesLabel], ",")
	for _, c := range relativePathConfigFiles {
		configFiles = append(configFiles, filepath.Base(c))
	}
	return cli.NewProjectOptions(configFiles,
		cli.WithOsEnv,
		cli.WithWorkingDirectory(c.Labels[workingDirLabel]),
		cli.WithName(c.Labels[projectLabel]))
}
