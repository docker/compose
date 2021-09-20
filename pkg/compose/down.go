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
	"strings"
	"time"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/errdefs"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

type downOp func() error

func (s *composeService) Down(ctx context.Context, projectName string, options api.DownOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.down(ctx, strings.ToLower(projectName), options)
	})
}

func (s *composeService) down(ctx context.Context, projectName string, options api.DownOptions) error {
	w := progress.ContextWriter(ctx)
	resourceToRemove := false

	var containers Containers
	containers, err := s.getContainers(ctx, projectName, oneOffInclude, true)
	if err != nil {
		return err
	}

	if options.Project == nil {
		options.Project = s.projectFromContainerLabels(containers.filter(isNotOneOff), projectName)
	}

	if len(containers) > 0 {
		resourceToRemove = true
	}

	err = InReverseDependencyOrder(ctx, options.Project, func(c context.Context, service string) error {
		serviceContainers := containers.filter(isService(service))
		err := s.removeContainers(ctx, w, serviceContainers, options.Timeout, options.Volumes)
		return err
	})
	if err != nil {
		return err
	}

	orphans := containers.filter(isNotService(options.Project.ServiceNames()...))
	if options.RemoveOrphans && len(orphans) > 0 {
		err := s.removeContainers(ctx, w, orphans, options.Timeout, false)
		if err != nil {
			return err
		}
	}

	ops, err := s.ensureNetwoksDown(ctx, projectName)
	if err != nil {
		return err
	}

	if options.Images != "" {
		ops = append(ops, s.ensureImagesDown(ctx, projectName, options, w)...)
	}

	if options.Volumes {
		rm, err := s.ensureVolumesDown(ctx, projectName, w)
		if err != nil {
			return err
		}
		ops = append(ops, rm...)
	}

	if !resourceToRemove && len(ops) == 0 {
		w.Event(progress.NewEvent(projectName, progress.Done, "Warning: No resource found to remove"))
	}

	eg, _ := errgroup.WithContext(ctx)
	for _, op := range ops {
		eg.Go(op)
	}
	return eg.Wait()
}

func (s *composeService) ensureVolumesDown(ctx context.Context, projectName string, w progress.Writer) ([]downOp, error) {
	var ops []downOp
	volumes, err := s.apiClient.VolumeList(ctx, filters.NewArgs(projectFilter(projectName)))
	if err != nil {
		return ops, err
	}
	for _, vol := range volumes.Volumes {
		id := vol.Name
		ops = append(ops, func() error {
			return s.removeVolume(ctx, id, w)
		})
	}
	return ops, nil
}

func (s *composeService) ensureImagesDown(ctx context.Context, projectName string, options api.DownOptions, w progress.Writer) []downOp {
	var ops []downOp
	for image := range s.getServiceImages(options, projectName) {
		image := image
		ops = append(ops, func() error {
			return s.removeImage(ctx, image, w)
		})
	}
	return ops
}

func (s *composeService) ensureNetwoksDown(ctx context.Context, projectName string) ([]downOp, error) {
	var ops []downOp
	networks, err := s.apiClient.NetworkList(ctx, moby.NetworkListOptions{Filters: filters.NewArgs(projectFilter(projectName))})
	if err != nil {
		return ops, err
	}
	for _, n := range networks {
		networkID := n.ID
		networkName := n.Name
		ops = append(ops, func() error {
			return s.removeNetwork(ctx, networkID, networkName)
		})
	}
	return ops, nil
}

func (s *composeService) getServiceImages(options api.DownOptions, projectName string) map[string]struct{} {
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

func (s *composeService) removeImage(ctx context.Context, image string, w progress.Writer) error {
	id := fmt.Sprintf("Image %s", image)
	w.Event(progress.NewEvent(id, progress.Working, "Removing"))
	_, err := s.apiClient.ImageRemove(ctx, image, moby.ImageRemoveOptions{})
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

func (s *composeService) removeVolume(ctx context.Context, id string, w progress.Writer) error {
	resource := fmt.Sprintf("Volume %s", id)
	w.Event(progress.NewEvent(resource, progress.Working, "Removing"))
	err := s.apiClient.VolumeRemove(ctx, id, true)
	if err == nil {
		w.Event(progress.NewEvent(resource, progress.Done, "Removed"))
		return nil
	}
	if errdefs.IsNotFound(err) {
		w.Event(progress.NewEvent(resource, progress.Done, "Warning: No resource found to remove"))
		return nil
	}
	return err
}

func (s *composeService) stopContainers(ctx context.Context, w progress.Writer, containers []moby.Container, timeout *time.Duration) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			eventName := getContainerProgressName(container)
			w.Event(progress.StoppingEvent(eventName))
			err := s.apiClient.ContainerStop(ctx, container.ID, timeout)
			if err != nil {
				w.Event(progress.ErrorMessageEvent(eventName, "Error while Stopping"))
				return err
			}
			w.Event(progress.StoppedEvent(eventName))
			return nil
		})
	}
	return eg.Wait()
}

func (s *composeService) removeContainers(ctx context.Context, w progress.Writer, containers []moby.Container, timeout *time.Duration, volumes bool) error {
	eg, _ := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			eventName := getContainerProgressName(container)
			w.Event(progress.StoppingEvent(eventName))
			err := s.stopContainers(ctx, w, []moby.Container{container}, timeout)
			if err != nil {
				w.Event(progress.ErrorMessageEvent(eventName, "Error while Stopping"))
				return err
			}
			w.Event(progress.RemovingEvent(eventName))
			err = s.apiClient.ContainerRemove(ctx, container.ID, moby.ContainerRemoveOptions{
				Force:         true,
				RemoveVolumes: volumes,
			})
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

func (s *composeService) projectFromContainerLabels(containers Containers, projectName string) *types.Project {
	project := &types.Project{
		Name: projectName,
	}
	if len(containers) == 0 {
		return project
	}
	set := map[string]moby.Container{}
	for _, c := range containers {
		set[c.Labels[api.ServiceLabel]] = c
	}
	for s, c := range set {
		service := types.ServiceConfig{
			Name:   s,
			Image:  c.Image,
			Labels: c.Labels,
		}
		dependencies := c.Labels[api.DependenciesLabel]
		if len(dependencies) > 0 {
			service.DependsOn = types.DependsOnConfig{}
			for _, d := range strings.Split(dependencies, ",") {
				service.DependsOn[d] = types.ServiceDependency{}
			}
		}
		project.Services = append(project.Services, service)
	}
	return project
}
