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

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/utils"
)

func (s *composeService) Images(ctx context.Context, projectName string, options compose.ImagesOptions) ([]compose.ImageSummary, error) {
	allContainers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	if err != nil {
		return nil, err
	}
	containers := []moby.Container{}
	if len(options.Services) > 0 {
		// filter service containers
		for _, c := range allContainers {
			if utils.StringContains(options.Services, c.Labels[compose.ServiceTag]) {
				containers = append(containers, c)

			}
		}
	} else {
		containers = allContainers
	}

	imageIDs := []string{}
	// aggregate image IDs
	for _, c := range containers {
		if !utils.StringContains(imageIDs, c.ImageID) {
			imageIDs = append(imageIDs, c.ImageID)
		}
	}

	images := map[string]moby.ImageInspect{}
	eg, ctx := errgroup.WithContext(ctx)
	for _, img := range imageIDs {
		img := img
		eg.Go(func() error {
			inspect, _, err := s.apiClient.ImageInspectWithRaw(ctx, img)
			if err != nil {
				return err
			}
			images[img] = inspect
			return nil
		})
	}
	err = eg.Wait()

	if err != nil {
		return nil, err
	}
	summary := make([]compose.ImageSummary, len(containers))
	for i, container := range containers {
		img, ok := images[container.ImageID]
		if !ok {
			return nil, fmt.Errorf("failed to retrieve image for container %s", getCanonicalContainerName(container))
		}
		if len(img.RepoTags) == 0 {
			return nil, fmt.Errorf("no image tag found for %s", img.ID)
		}
		tag := ""
		repository := ""
		repotag := strings.Split(img.RepoTags[0], ":")
		repository = repotag[0]
		if len(repotag) > 1 {
			tag = repotag[1]
		}

		summary[i] = compose.ImageSummary{
			ID:            img.ID,
			ContainerName: getCanonicalContainerName(container),
			Repository:    repository,
			Tag:           tag,
			Size:          img.Size,
		}
	}
	return summary, nil
}
