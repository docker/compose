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
	"github.com/docker/docker/errdefs"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/compose-cli/api/compose"

	"github.com/docker/compose-cli/api/progress"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	w := progress.ContextWriter(ctx)
	resourceToRemove := false

	if options.Project == nil {
		project, err := s.projectFromContainerLabels(ctx, projectName)
		if err != nil {
			return err
		}
		options.Project = project
	}

	var containers Containers
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(options.Project.Name)),
		All:     true,
	})
	if err != nil {
		return err
	}
	if len(containers) > 0 {
		resourceToRemove = true
	}

	err = InReverseDependencyOrder(ctx, options.Project, func(c context.Context, service types.ServiceConfig) error {
		serviceContainers := containers.filter(isService(service.Name))
		err := s.removeContainers(ctx, w, serviceContainers, options.Timeout)
		return err
	})
	if err != nil {
		return err
	}

	orphans := containers.filter(isNotService(options.Project.ServiceNames()...))
	if options.RemoveOrphans && len(orphans) > 0 {
		err := s.removeContainers(ctx, w, orphans, options.Timeout)
		if err != nil {
			return err
		}
	}

	networks, err := s.apiClient.NetworkList(ctx, moby.NetworkListOptions{Filters: filters.NewArgs(projectFilter(projectName))})
	if err != nil {
		return err
	}

	eg, _ := errgroup.WithContext(ctx)
	for _, n := range networks {
		resourceToRemove = true
		networkID := n.ID
		networkName := n.Name
		eg.Go(func() error {
			return s.ensureNetworkDown(ctx, networkID, networkName)
		})
	}

	if options.Images != "" {
		for image := range s.getServiceImages(options, projectName) {
			image := image
			eg.Go(func() error {
				resourceToRemove = true
				return s.removeImage(image, w, err, ctx)
			})
		}
	}

	if !resourceToRemove {
		w.Event(progress.NewEvent(projectName, progress.Done, "Warning: No resource found to remove"))
	}
	return eg.Wait()
}

func (s *composeService) getServiceImages(options compose.DownOptions, projectName string) map[string]struct{} {
	images := map[string]struct{}{}
	for _, service := range options.Project.Services {
		image := service.Image
		if options.Images == "local" && image != "" {
			continue
		}
		if image == "" {
			image = getImageName(service, projectName)
		}
		images[image] = struct{}{}
	}
	return images
}

func (s *composeService) removeImage(image string, w progress.Writer, err error, ctx context.Context) error {
	id := fmt.Sprintf("Image %s", image)
	w.Event(progress.NewEvent(id, progress.Working, "Removing"))
	_, err = s.apiClient.ImageRemove(ctx, image, moby.ImageRemoveOptions{})
	if err == nil {
		w.Event(progress.NewEvent(id, progress.Done, "Removed"))
		return nil
	}
	if errdefs.IsNotFound(err) {
		w.Event(progress.NewEvent(id, progress.Done, "Warning: No resource found to remove"))
		return nil
	}
	return err
}

func (s *composeService) stopContainers(ctx context.Context, w progress.Writer, containers []moby.Container, timeout *time.Duration) error {
	for _, container := range containers {
		toStop := container
		eventName := getContainerProgressName(toStop)
		w.Event(progress.StoppingEvent(eventName))
		err := s.apiClient.ContainerStop(ctx, toStop.ID, timeout)
		if err != nil {
			w.Event(progress.ErrorMessageEvent(eventName, "Error while Stopping"))
			return err
		}
		w.Event(progress.StoppedEvent(eventName))
	}
	return nil
}

func (s *composeService) removeContainers(ctx context.Context, w progress.Writer, containers []moby.Container, timeout *time.Duration) error {
	eg, _ := errgroup.WithContext(ctx)
	for _, container := range containers {
		toDelete := container
		eg.Go(func() error {
			eventName := getContainerProgressName(toDelete)
			w.Event(progress.StoppingEvent(eventName))
			err := s.stopContainers(ctx, w, []moby.Container{toDelete}, timeout)
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
	return eg.Wait()
}

func projectFilterListOpt(projectName string) moby.ContainerListOptions {
	return moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
		All:     true,
	}
}

func (s *composeService) projectFromContainerLabels(ctx context.Context, projectName string) (*types.Project, error) {
	containers, err := s.apiClient.ContainerList(ctx, projectFilterListOpt(projectName))
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
