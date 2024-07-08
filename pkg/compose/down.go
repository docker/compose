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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/internal/desktop"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imageapi "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"golang.org/x/sync/errgroup"
)

type downOp func() error

func (s *composeService) Down(ctx context.Context, projectName string, options api.DownOptions) error {
	return progress.Run(ctx, func(ctx context.Context) error {
		return s.down(ctx, strings.ToLower(projectName), options)
	}, s.stdinfo())
}

func (s *composeService) down(ctx context.Context, projectName string, options api.DownOptions) error { //nolint:gocyclo
	w := progress.ContextWriter(ctx)
	resourceToRemove := false

	include := oneOffExclude
	if options.RemoveOrphans {
		include = oneOffInclude
	}
	containers, err := s.getContainers(ctx, projectName, include, true)
	if err != nil {
		return err
	}

	project := options.Project
	if project == nil {
		project, err = s.getProjectWithResources(ctx, containers, projectName)
		if err != nil {
			return err
		}
	}

	// Check requested services exists in model
	options.Services, err = checkSelectedServices(options, project)
	if err != nil {
		return err
	}

	if len(containers) > 0 {
		resourceToRemove = true
	}

	err = InReverseDependencyOrder(ctx, project, func(c context.Context, service string) error {
		serviceContainers := containers.filter(isService(service))
		err := s.removeContainers(ctx, serviceContainers, options.Timeout, options.Volumes)
		return err
	}, WithRootNodesAndDown(options.Services))
	if err != nil {
		return err
	}

	orphans := containers.filter(isOrphaned(project))
	if options.RemoveOrphans && len(orphans) > 0 {
		err := s.removeContainers(ctx, orphans, options.Timeout, false)
		if err != nil {
			return err
		}
	}

	ops := s.ensureNetworksDown(ctx, project, w)

	if options.Images != "" {
		imgOps, err := s.ensureImagesDown(ctx, project, options, w)
		if err != nil {
			return err
		}
		ops = append(ops, imgOps...)
	}

	if options.Volumes {
		ops = append(ops, s.ensureVolumesDown(ctx, project, w)...)
	}

	if !resourceToRemove && len(ops) == 0 {
		fmt.Fprintf(s.stderr(), "Warning: No resource found to remove for project %q.\n", projectName)
	}

	eg, _ := errgroup.WithContext(ctx)
	for _, op := range ops {
		eg.Go(op)
	}
	return eg.Wait()
}

func checkSelectedServices(options api.DownOptions, project *types.Project) ([]string, error) {
	var services []string
	for _, service := range options.Services {
		_, err := project.GetService(service)
		if err != nil {
			if options.Project != nil {
				// ran with an explicit compose.yaml file, so we should not ignore
				return nil, err
			}
			// ran without an explicit compose.yaml file, so can't distinguish typo vs container already removed
		} else {
			services = append(services, service)
		}
	}
	return services, nil
}

func (s *composeService) ensureVolumesDown(ctx context.Context, project *types.Project, w progress.Writer) []downOp {
	var ops []downOp
	for _, vol := range project.Volumes {
		if vol.External {
			continue
		}
		volumeName := vol.Name
		ops = append(ops, func() error {
			return s.removeVolume(ctx, volumeName, w)
		})
	}

	if s.manageDesktopFileSharesEnabled(ctx) {
		ops = append(ops, func() error {
			desktop.RemoveFileSharesForProject(ctx, s.desktopCli, project.Name)
			return nil
		})
	}

	return ops
}

func (s *composeService) ensureImagesDown(ctx context.Context, project *types.Project, options api.DownOptions, w progress.Writer) ([]downOp, error) {
	imagePruner := NewImagePruner(s.apiClient(), project)
	pruneOpts := ImagePruneOptions{
		Mode:          ImagePruneMode(options.Images),
		RemoveOrphans: options.RemoveOrphans,
	}
	images, err := imagePruner.ImagesToPrune(ctx, pruneOpts)
	if err != nil {
		return nil, err
	}

	var ops []downOp
	for i := range images {
		img := images[i]
		ops = append(ops, func() error {
			return s.removeImage(ctx, img, w)
		})
	}
	return ops, nil
}

func (s *composeService) ensureNetworksDown(ctx context.Context, project *types.Project, w progress.Writer) []downOp {
	var ops []downOp
	for key, n := range project.Networks {
		if n.External {
			continue
		}
		// loop capture variable for op closure
		networkKey := key
		idOrName := n.Name
		ops = append(ops, func() error {
			return s.removeNetwork(ctx, networkKey, project.Name, idOrName, w)
		})
	}
	return ops
}

func (s *composeService) removeNetwork(ctx context.Context, composeNetworkName string, projectName string, name string, w progress.Writer) error {
	networks, err := s.apiClient().NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
			networkFilter(composeNetworkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	if len(networks) == 0 {
		return nil
	}

	eventName := fmt.Sprintf("Network %s", name)
	w.Event(progress.RemovingEvent(eventName))

	var found int
	for _, net := range networks {
		if net.Name != name {
			continue
		}
		nw, err := s.apiClient().NetworkInspect(ctx, net.ID, network.InspectOptions{})
		if errdefs.IsNotFound(err) {
			w.Event(progress.NewEvent(eventName, progress.Warning, "No resource found to remove"))
			return nil
		}
		if err != nil {
			return err
		}
		if len(nw.Containers) > 0 {
			w.Event(progress.NewEvent(eventName, progress.Warning, "Resource is still in use"))
			found++
			continue
		}

		if err := s.apiClient().NetworkRemove(ctx, net.ID); err != nil {
			if errdefs.IsNotFound(err) {
				continue
			}
			w.Event(progress.ErrorEvent(eventName))
			return fmt.Errorf("failed to remove network %s: %w", name, err)
		}
		w.Event(progress.RemovedEvent(eventName))
		found++
	}

	if found == 0 {
		// in practice, it's extremely unlikely for this to ever occur, as it'd
		// mean the network was present when we queried at the start of this
		// method but was then deleted by something else in the interim
		w.Event(progress.NewEvent(eventName, progress.Warning, "No resource found to remove"))
		return nil
	}
	return nil
}

func (s *composeService) removeImage(ctx context.Context, image string, w progress.Writer) error {
	id := fmt.Sprintf("Image %s", image)
	w.Event(progress.NewEvent(id, progress.Working, "Removing"))
	_, err := s.apiClient().ImageRemove(ctx, image, imageapi.RemoveOptions{})
	if err == nil {
		w.Event(progress.NewEvent(id, progress.Done, "Removed"))
		return nil
	}
	if errdefs.IsConflict(err) {
		w.Event(progress.NewEvent(id, progress.Warning, "Resource is still in use"))
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
	err := s.apiClient().VolumeRemove(ctx, id, true)
	if err == nil {
		w.Event(progress.NewEvent(resource, progress.Done, "Removed"))
		return nil
	}
	if errdefs.IsConflict(err) {
		w.Event(progress.NewEvent(resource, progress.Warning, "Resource is still in use"))
		return nil
	}
	if errdefs.IsNotFound(err) {
		w.Event(progress.NewEvent(resource, progress.Done, "Warning: No resource found to remove"))
		return nil
	}
	return err
}

func (s *composeService) stopContainer(ctx context.Context, w progress.Writer, container moby.Container, timeout *time.Duration) error {
	eventName := getContainerProgressName(container)
	w.Event(progress.StoppingEvent(eventName))
	timeoutInSecond := utils.DurationSecondToInt(timeout)
	err := s.apiClient().ContainerStop(ctx, container.ID, containerType.StopOptions{Timeout: timeoutInSecond})
	if err != nil {
		w.Event(progress.ErrorMessageEvent(eventName, "Error while Stopping"))
		return err
	}
	w.Event(progress.StoppedEvent(eventName))
	return nil
}

func (s *composeService) stopContainers(ctx context.Context, w progress.Writer, containers []moby.Container, timeout *time.Duration) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			return s.stopContainer(ctx, w, container, timeout)
		})
	}
	return eg.Wait()
}

func (s *composeService) removeContainers(ctx context.Context, containers []moby.Container, timeout *time.Duration, volumes bool) error {
	eg, _ := errgroup.WithContext(ctx)
	for _, container := range containers {
		container := container
		eg.Go(func() error {
			return s.stopAndRemoveContainer(ctx, container, timeout, volumes)
		})
	}
	return eg.Wait()
}

func (s *composeService) stopAndRemoveContainer(ctx context.Context, container moby.Container, timeout *time.Duration, volumes bool) error {
	w := progress.ContextWriter(ctx)
	eventName := getContainerProgressName(container)
	err := s.stopContainer(ctx, w, container, timeout)
	if errdefs.IsNotFound(err) {
		w.Event(progress.RemovedEvent(eventName))
		return nil
	}
	if err != nil {
		return err
	}
	w.Event(progress.RemovingEvent(eventName))
	err = s.apiClient().ContainerRemove(ctx, container.ID, containerType.RemoveOptions{
		Force:         true,
		RemoveVolumes: volumes,
	})
	if err != nil && !errdefs.IsNotFound(err) && !errdefs.IsConflict(err) {
		w.Event(progress.ErrorMessageEvent(eventName, "Error while Removing"))
		return err
	}
	w.Event(progress.RemovedEvent(eventName))
	return nil
}

func (s *composeService) getProjectWithResources(ctx context.Context, containers Containers, projectName string) (*types.Project, error) {
	containers = containers.filter(isNotOneOff)
	p, err := s.projectFromName(containers, projectName)
	if err != nil && !api.IsNotFoundError(err) {
		return nil, err
	}
	project, err := p.WithServicesTransform(func(name string, service types.ServiceConfig) (types.ServiceConfig, error) {
		for k := range service.DependsOn {
			if dependency, ok := service.DependsOn[k]; ok {
				dependency.Required = false
				service.DependsOn[k] = dependency
			}
		}
		return service, nil
	})
	if err != nil {
		return nil, err
	}

	volumes, err := s.actualVolumes(ctx, projectName)
	if err != nil {
		return nil, err
	}
	project.Volumes = volumes

	networks, err := s.actualNetworks(ctx, projectName)
	if err != nil {
		return nil, err
	}
	project.Networks = networks

	return project, nil
}
