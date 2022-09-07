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
	"github.com/distribution/distribution/v3/reference"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/errdefs"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
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
	images, err := s.getServiceImagesToRemove(ctx, options, project)
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

//nolint:gocyclo
func (s *composeService) getServiceImagesToRemove(ctx context.Context, options api.DownOptions, project *types.Project) ([]string, error) {
	if options.Images == "" {
		return nil, nil
	}

	var localServiceImages []string
	var imagesToRemove []string
	addImageToRemove := func(img string, checkExistence bool) {
		// since some references come from user input (service.image) and some
		// come from the engine API, we standardize them, opting for the
		// familiar name format since they'll also be displayed in the CLI
		ref, err := reference.ParseNormalizedNamed(img)
		if err != nil {
			return
		}
		ref = reference.TagNameOnly(ref)
		img = reference.FamiliarString(ref)
		if utils.StringContains(imagesToRemove, img) {
			return
		}

		if checkExistence {
			_, _, err := s.apiClient().ImageInspectWithRaw(ctx, img)
			if errdefs.IsNotFound(err) {
				// err on the side of caution: only skip if we successfully
				// queried the API and got back a definitive "not exists"
				return
			}
		}

		imagesToRemove = append(imagesToRemove, img)
	}

	imageListOpts := moby.ImageListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
			// TODO(milas): we should really clean up the dangling images as
			// well (historically we have NOT); need to refactor this to handle
			// it gracefully without producing confusing CLI output, i.e. we
			// do not want to print out a bunch of untagged/dangling image IDs,
			// they should be grouped into a logical operation for the relevant
			// service
			filters.Arg("dangling", "false"),
		),
	}
	projectImages, err := s.apiClient().ImageList(ctx, imageListOpts)
	if err != nil {
		return nil, err
	}

	// 1. Remote / custom-named images - only deleted on `--rmi="all"`
	for _, service := range project.Services {
		if service.Image == "" {
			localServiceImages = append(localServiceImages, service.Name)
			continue
		}

		if options.Images == "all" {
			addImageToRemove(service.Image, true)
		}
	}

	// 2. *LABELED* Locally-built images with implicit image names
	//
	// If `--remove-orphans` is being used, then ALL images for the project
	// will be selected for removal. Otherwise, only those that match a known
	// service based on the loaded project will be included.
	for _, img := range projectImages {
		if len(img.RepoTags) == 0 {
			// currently, we're only removing the tagged references, but
			// if we start removing the dangling images and grouping by
			// service, we can remove this (and should rely on `Image::ID`)
			continue
		}

		shouldRemove := options.RemoveOrphans
		for _, service := range localServiceImages {
			if img.Labels[api.ServiceLabel] == service {
				shouldRemove = true
				break
			}
		}

		if shouldRemove {
			addImageToRemove(img.RepoTags[0], false)
		}
	}

	// 3. *UNLABELED* Locally-built images with implicit image names
	//
	// This is a fallback for (2) to handle images built by previous
	// versions of Compose, which did not label their built images.
	for _, serviceName := range localServiceImages {
		service, err := project.GetService(serviceName)
		if err != nil || service.Image != "" {
			continue
		}
		imgName := api.GetImageNameOrDefault(service, project.Name)
		addImageToRemove(imgName, true)
	}
	return imagesToRemove, nil
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
