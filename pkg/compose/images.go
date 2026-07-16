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
	"time"

	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Images(ctx context.Context, projectName string, options api.ImagesOptions) (map[string]api.ImageSummary, error) {
	projectName = strings.ToLower(projectName)
	allContainers, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: projectFilter(projectName),
	})
	if err != nil {
		return nil, err
	}
	var containers []container.Summary
	if len(options.Services) > 0 {
		// filter service containers
		for _, c := range allContainers.Items {
			if slices.Contains(options.Services, c.Labels[api.ServiceLabel]) {
				containers = append(containers, c)
			}
		}
	} else {
		containers = allContainers.Items
	}

	// The daemon validates the platform field in ImageInspect against the
	// negotiated API version from the request path, not the server's own max version.
	version, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return nil, err
	}
	withPlatform := versions.GreaterThanOrEqualTo(version, apiVersion149)

	summary := map[string]api.ImageSummary{}
	var mux sync.Mutex
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		eg.Go(func() error {
			img, err := s.apiClient().ImageInspect(ctx, c.Image)
			if err != nil {
				return err
			}
			id := img.ID // platform-specific image ID can't be combined with image tag, see https://github.com/moby/moby/issues/49995

			if withPlatform && c.ImageManifestDescriptor != nil && c.ImageManifestDescriptor.Platform != nil {
				img, err = s.apiClient().ImageInspect(ctx, c.Image, client.ImageInspectWithPlatform(c.ImageManifestDescriptor.Platform))
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

			var created *time.Time
			if img.Created != "" {
				t, err := time.Parse(time.RFC3339Nano, img.Created)
				if err != nil {
					return err
				}
				created = &t
			}

			mux.Lock()
			defer mux.Unlock()
			summary[getCanonicalContainerName(c)] = api.ImageSummary{
				ID:         id,
				Repository: repository,
				Tag:        tag,
				Platform: platforms.Platform{
					Architecture: img.Architecture,
					OS:           img.Os,
					OSVersion:    img.OsVersion,
					Variant:      img.Variant,
				},
				Size:        img.Size,
				Created:     created,
				LastTagTime: img.Metadata.LastTagTime,
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

	version, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return nil, err
	}
	withManifests := versions.GreaterThanOrEqualTo(version, apiVersion148)

	eg, ctx := errgroup.WithContext(ctx)
	for _, repoTag := range repoTags {
		eg.Go(func() error {
			var opts []client.ImageInspectOption
			if withManifests {
				opts = append(opts, client.ImageInspectWithManifests(true))
			}
			inspect, err := s.apiClient().ImageInspect(ctx, repoTag, opts...)
			if err != nil {
				if errdefs.IsNotFound(err) {
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
				ID:          contentDigest(inspect.InspectResponse),
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

// contentDigest returns the digest identifying an image's actual content
// (config + layers). When BuildKit provenance attestations are enabled, the
// image is stored as a multi-manifest index whose top-level digest
// (inspect.ID) also covers the attestation manifest and therefore changes on
// every build even when the runnable image itself is unchanged. Reading the
// "image" kind manifest instead gives a digest that only reflects the image
// content, matching what compose actually needs to detect staleness.
func contentDigest(inspect image.InspectResponse) string {
	for _, m := range inspect.Manifests {
		if m.Kind == image.ManifestKindImage {
			return m.ID
		}
	}
	return inspect.ID
}
