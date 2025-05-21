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
	"slices"
	"strings"
	"sync"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
)

func (s *composeService) Images(ctx context.Context, projectName string, options api.ImagesOptions) (map[string]api.ImageSummary, error) {
	projectName = strings.ToLower(projectName)
	allContainers, err := s.apiClient().ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	if err != nil {
		return nil, err
	}
	var containers []container.Summary
	if len(options.Services) > 0 {
		// filter service containers
		for _, c := range allContainers {
			if slices.Contains(options.Services, c.Labels[api.ServiceLabel]) {
				containers = append(containers, c)
			}
		}
	} else {
		containers = allContainers
	}

	version, err := s.RuntimeVersion(ctx)
	if err != nil {
		return nil, err
	}
	withPlatform := versions.GreaterThanOrEqualTo(version, "1.49")

	summary := map[string]api.ImageSummary{}
	var mux sync.Mutex
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		eg.Go(func() error {
			image, err := s.apiClient().ImageInspect(ctx, c.Image)
			if err != nil {
				return err
			}
			id := image.ID // platform-specific image ID can't be combined with image tag, see https://github.com/moby/moby/issues/49995

			if withPlatform && c.ImageManifestDescriptor != nil && c.ImageManifestDescriptor.Platform != nil {
				image, err = s.apiClient().ImageInspect(ctx, c.Image, client.ImageInspectWithPlatform(c.ImageManifestDescriptor.Platform))
				if err != nil {
					return err
				}
			}

			var repository, tag string
			ref, err := reference.ParseDockerRef(c.Image)
			if err == nil {
				// ParseDockerRef will reject a local image ID
				repository = reference.FamiliarName(ref)
				if tagged, ok := ref.(reference.Tagged); ok {
					tag = tagged.Tag()
				}
			}

			mux.Lock()
			defer mux.Unlock()
			summary[getCanonicalContainerName(c)] = api.ImageSummary{
				ID:         id,
				Repository: repository,
				Tag:        tag,
				Platform: platforms.Platform{
					Architecture: image.Architecture,
					OS:           image.Os,
					OSVersion:    image.OsVersion,
					Variant:      image.Variant,
				},
				Size:        image.Size,
				LastTagTime: image.Metadata.LastTagTime,
			}
			return nil
		})
	}

	err = eg.Wait()
	return summary, err
}

func (s *composeService) getImageSummaries(ctx context.Context, repoTags []string) (map[string]api.ImageSummary, error) {
	summary := map[string]api.ImageSummary{}
	l := sync.Mutex{}
	eg, ctx := errgroup.WithContext(ctx)
	for _, repoTag := range repoTags {
		eg.Go(func() error {
			inspect, err := s.apiClient().ImageInspect(ctx, repoTag)
			if err != nil {
				if cerrdefs.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("unable to get image '%s': %w", repoTag, err)
			}
			tag := ""
			repository := ""
			ref, err := reference.ParseDockerRef(repoTag)
			if err == nil {
				// ParseDockerRef will reject a local image ID
				repository = reference.FamiliarName(ref)
				if tagged, ok := ref.(reference.Tagged); ok {
					tag = tagged.Tag()
				}
			}
			l.Lock()
			summary[repoTag] = api.ImageSummary{
				ID:          inspect.ID,
				Repository:  repository,
				Tag:         tag,
				Size:        inspect.Size,
				LastTagTime: inspect.Metadata.LastTagTime,
			}
			l.Unlock()
			return nil
		})
	}
	return summary, eg.Wait()
}
