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
	"sync"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/errdefs"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) Images(ctx context.Context, projectName string, options api.ImagesOptions) ([]api.ImageSummary, error) {
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
			if utils.StringContains(options.Services, c.Labels[api.ServiceLabel]) {
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
	images, err := s.getImages(ctx, imageIDs)
	if err != nil {
		return nil, err
	}
	summary := make([]api.ImageSummary, len(containers))
	for i, container := range containers {
		img, ok := images[container.ImageID]
		if !ok {
			return nil, fmt.Errorf("failed to retrieve image for container %s", getCanonicalContainerName(container))
		}

		summary[i] = img
		summary[i].ContainerName = getCanonicalContainerName(container)
	}
	return summary, nil
}

func (s *composeService) getImages(ctx context.Context, images []string) (map[string]api.ImageSummary, error) {
	summary := map[string]api.ImageSummary{}
	l := sync.Mutex{}
	eg, ctx := errgroup.WithContext(ctx)
	for _, img := range images {
		img := img
		eg.Go(func() error {
			inspect, _, err := s.apiClient.ImageInspectWithRaw(ctx, img)
			if err != nil {
				if errdefs.IsNotFound(err) {
					return nil
				}
				return err
			}
			tag := ""
			repository := ""
			if len(inspect.RepoTags) > 0 {

				repotag := strings.Split(inspect.RepoTags[0], ":")
				repository = repotag[0]
				if len(repotag) > 1 {
					tag = repotag[1]
				}
			}
			l.Lock()
			summary[img] = api.ImageSummary{
				ID:         inspect.ID,
				Repository: repository,
				Tag:        tag,
				Size:       inspect.Size,
			}
			l.Unlock()
			return nil
		})
	}
	return summary, eg.Wait()
}
