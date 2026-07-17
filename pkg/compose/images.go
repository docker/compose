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

	withManifests, err := s.manifestsSupported(ctx)
	if err != nil {
		return nil, err
	}

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
				ID:          contentDigest(inspect.InspectResponse, platforms.Default()),
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

// manifestsSupported reports whether the engine can return per-manifest data on
// image inspect (Engine >= 28.0 / API >= 1.48). Older engines fall back to the
// plain image ID.
func (s *composeService) manifestsSupported(ctx context.Context) (bool, error) {
	version, err := s.RuntimeAPIVersion(ctx)
	if err != nil {
		return false, err
	}
	return versions.GreaterThanOrEqualTo(version, apiVersion148), nil
}

// contentDigest returns the digest identifying an image's runnable content
// (config + layers) for the given platform. With BuildKit provenance
// attestations enabled (the default since recent Buildx/BuildKit), the image is
// stored as an index whose top-level digest (inspect.ID) also covers the
// attestation manifest, so it changes on every build even when the runnable
// image is unchanged — making compose recreate containers needlessly (see
// https://github.com/docker/compose/issues/13636). The digest of the "image"
// kind manifest reflects only the image content, which is what compose needs to
// detect staleness.
//
// Selection is platform-aware and deterministic so the same image always maps
// to the same digest across rebuilds: the available manifest matching the
// requested platform wins; a lone available image manifest is used as-is
// (single-platform images, whatever their platform); otherwise we fall back to
// inspect.ID (engines that don't report manifests — where inspect.ID is already
// the config digest — or a multi-platform image whose requested platform isn't
// available locally, which the caller then treats as a platform miss).
func contentDigest(inspect image.InspectResponse, platform platforms.MatchComparer) string {
	var available []image.ManifestSummary
	for _, m := range inspect.Manifests {
		if m.Kind == image.ManifestKindImage && m.Available {
			available = append(available, m)
		}
	}
	for _, m := range available {
		if m.ImageData != nil && platform.Match(m.ImageData.Platform) {
			return m.ID
		}
	}
	if len(available) == 1 {
		return available[0].ID
	}
	return inspect.ID
}
