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
	"github.com/pkg/errors"
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

	if len(containers) > 0 {
		resourceToRemove = true
	}

	err = InReverseDependencyOrder(ctx, project, func(c context.Context, service string) error {
		serviceContainers := containers.filter(isService(service))
		err := s.removeContainers(ctx, w, serviceContainers, options.Timeout, options.Volumes)
		return err
	})
	if err != nil {
		return err
	}

	orphans := containers.filter(isNotService(project.ServiceNames()...))
	if options.RemoveOrphans && len(orphans) > 0 {
		err := s.removeContainers(ctx, w, orphans, options.Timeout, false)
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

func (s *composeService) ensureVolumesDown(ctx context.Context, project *types.Project, w progress.Writer) []downOp {
	var ops []downOp
	for _, vol := range project.Volumes {
		if vol.External.External {
			continue
		}
		volumeName := vol.Name
		ops = append(ops, func() error {
			return s.removeVolume(ctx, volumeName, w)
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
	for _, n := range project.Networks {
		if n.External.External {
			continue
		}
		// loop capture variable for op closure
		networkName := n.Name
		ops = append(ops, func() error {
			return s.removeNetwork(ctx, networkName, w)
		})
	}
	return ops
}

func (s *composeService) removeNetwork(ctx context.Context, name string, w progress.Writer) error {
	// networks are guaranteed to have unique IDs but NOT names, so it's
	// possible to get into a situation where a compose down will fail with
	// an error along the lines of:
	// 	failed to remove network test: Error response from daemon: network test is ambiguous (2 matches found based on name)
	// as a workaround here, the delete is done by ID after doing a list using
	// the name as a filter (99.9% of the time this will return a single result)
	networks, err := s.apiClient().NetworkList(ctx, moby.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return errors.Wrapf(err, fmt.Sprintf("failed to inspect network %s", name))
	}
	if len(networks) == 0 {
		return nil
	}

	eventName := fmt.Sprintf("Network %s", name)
	w.Event(progress.RemovingEvent(eventName))

	var removed int
	for _, net := range networks {
		if net.Name == name {
			if err := s.apiClient().NetworkRemove(ctx, net.ID); err != nil {
				if errdefs.IsNotFound(err) {
					continue
				}
				w.Event(progress.ErrorEvent(eventName))
				return errors.Wrapf(err, fmt.Sprintf("failed to remove network %s", name))
			}
			removed++
		}
	}

	if removed == 0 {
		// in practice, it's extremely unlikely for this to ever occur, as it'd
		// mean the network was present when we queried at the start of this
		// method but was then deleted by something else in the interim
		w.Event(progress.NewEvent(eventName, progress.Done, "Warning: No resource found to remove"))
		return nil
	}

	w.Event(progress.RemovedEvent(eventName))
	return nil
}

func (s *composeService) removeImage(ctx context.Context, image string, w progress.Writer) error {
	id := fmt.Sprintf("Image %s", image)
	w.Event(progress.NewEvent(id, progress.Working, "Removing"))
	_, err := s.apiClient().ImageRemove(ctx, image, moby.ImageRemoveOptions{})
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
	err := s.apiClient().VolumeRemove(ctx, id, true)
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
			err := s.apiClient().ContainerStop(ctx, container.ID, timeout)
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
			err = s.apiClient().ContainerRemove(ctx, container.ID, moby.ContainerRemoveOptions{
				Force:         true,
				RemoveVolumes: volumes,
			})
			if err != nil && !errdefs.IsNotFound(err) {
				w.Event(progress.ErrorMessageEvent(eventName, "Error while Removing"))
				return err
			}
			w.Event(progress.RemovedEvent(eventName))
			return nil
		})
	}
	return eg.Wait()
}

func (s *composeService) getProjectWithResources(ctx context.Context, containers Containers, projectName string) (*types.Project, error) {
	containers = containers.filter(isNotOneOff)
	project, err := s.projectFromName(containers, projectName)
	if err != nil && !api.IsNotFoundError(err) {
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
