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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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

// inspectLocalImages inspects the given references in parallel, requesting
// per-manifest data on engines that support it. References not found locally
// are simply absent from the result.
func (s *composeService) inspectLocalImages(ctx context.Context, repoTags []string) (map[string]client.ImageInspectResult, error) {
	opts, err := s.imageInspectOptions(ctx)
	if err != nil {
		return nil, err
	}
	inspections := map[string]client.ImageInspectResult{}
	l := sync.Mutex{}
	eg, ctx := errgroup.WithContext(ctx)
	for _, repoTag := range repoTags {
		eg.Go(func() error {
			inspect, err := s.apiClient().ImageInspect(ctx, repoTag, opts...)
			if err != nil {
				if errdefs.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("unable to get image '%s': %w", repoTag, err)
			}
			l.Lock()
			inspections[repoTag] = inspect
			l.Unlock()
			return nil
		})
	}
	return inspections, eg.Wait()
}

// imageInspectOptions requests per-manifest data when the engine supports it
// (see manifestsSupported).
func (s *composeService) imageInspectOptions(ctx context.Context) ([]client.ImageInspectOption, error) {
	withManifests, err := s.manifestsSupported(ctx)
	if err != nil {
		return nil, err
	}
	if !withManifests {
		return nil, nil
	}
	return []client.ImageInspectOption{client.ImageInspectWithManifests(true)}, nil
}

func imageSummary(repoTag string, inspect client.ImageInspectResult) api.ImageSummary {
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
	id, _, _ := localContentDigest(inspect, "")
	return api.ImageSummary{
		ID:          id,
		Repository:  repository,
		Tag:         tag,
		Size:        inspect.Size,
		LastTagTime: inspect.Metadata.LastTagTime,
	}
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

// serviceImageDigest returns the digest to record in a service's
// com.docker.compose.image label. A platform-pinned service uses the digest
// resolved in-process from the pre-pull inspect while it is still current;
// once the shared summary entry was refreshed by a pull or build during the
// run, that resolution is stale AND the refreshed digest was resolved for the
// platform of whichever service triggered the refresh — services sharing the
// image with a different pinned platform re-resolve theirs with one extra
// inspect (only in that refresh case; steady-state runs never get here). Best
// effort: the shared digest is kept when the pinned platform can't be
// satisfied or inspected.
func (s *composeService) serviceImageDigest(ctx context.Context, service types.ServiceConfig, imgName string, img api.ImageSummary, pinnedDigests map[string]pinnedImageDigest) string {
	if service.Platform == "" {
		return img.ID
	}
	if pinned, ok := pinnedDigests[service.Name]; ok && pinned.from == img.ID {
		return pinned.digest
	}
	digest, satisfied, err := s.inspectLocalContent(ctx, imgName, service.Platform)
	if err != nil || !satisfied {
		logrus.Debugf("unable to resolve %s for pinned platform %s, keeping shared digest: satisfied=%v err=%v", imgName, service.Platform, satisfied, err)
		return img.ID
	}
	return digest
}

// canonicalBuiltDigest resolves the canonical content digest of a just-built
// image so the recorded identity matches what later runs compute for the same
// local image. Registry-only builds (push-only, multi-platform without load)
// are not locally inspectable and keep the builder-reported digest: a volatile
// but honest value — an actual rebuild is still detected — preferred over a
// stable marker that would hide real image changes. Best effort: never fails
// an already-successful build.
func (s *composeService) canonicalBuiltDigest(ctx context.Context, imageRef, platform, builderDigest string) string {
	id, _, err := s.inspectLocalContent(ctx, imageRef, platform)
	if err != nil {
		logrus.Debugf("unable to resolve content digest for built image %s, keeping builder-reported digest: %v", imageRef, err)
		return builderDigest
	}
	return id
}

// localContentDigest returns the digest identifying an inspected image's
// runnable content (config + layers) for the requested platform (empty means
// the host default), plus whether the local image satisfies that platform.
// Note the satisfied bool is only meaningful for an explicitly requested
// platform: with platform empty, the manifests path reports host-default
// satisfaction while the manifest-less path always reports true.
//
// With BuildKit provenance attestations enabled (the default since recent
// Buildx/BuildKit), the image is stored as an index whose top-level digest
// (inspect.ID) also covers the attestation manifest, so it changes on every
// build even when the runnable image is unchanged — making compose recreate
// containers needlessly (see
// https://github.com/docker/compose/issues/13636). The digest of the "image"
// kind manifest reflects only the image content, which is what compose needs
// to detect staleness. Images inspected without manifest data (engines that
// can't report them) keep the plain image ID — already the config digest —
// with platform satisfaction from the inspect's flat platform fields, the
// pre-manifest behavior. Every image identity compose records for staleness
// comparison must be computed through here, whatever the image's provenance
// (pulled, built, already local), so any two runs produce comparable values.
func localContentDigest(inspect client.ImageInspectResult, platform string) (string, bool, error) {
	var matcher platforms.Matcher = platforms.Default()
	pinned := platform != ""
	if pinned {
		p, err := platforms.Parse(platform)
		if err != nil {
			return "", false, err
		}
		matcher = platforms.NewMatcher(p)
	}
	if len(inspect.Manifests) > 0 {
		id, ok := matchLocalManifest(inspect.InspectResponse, matcher)
		return id, ok, nil
	}
	ok := true
	if pinned {
		ok = matcher.Match(specs.Platform{
			Architecture: inspect.Architecture,
			OS:           inspect.Os,
			Variant:      inspect.Variant,
		})
	}
	return inspect.ID, ok, nil
}

// matchLocalManifest selects, among the locally available image manifests of
// inspect, the digest identifying the runnable content for the requested
// platform, and reports whether the local content actually satisfies that
// platform. Selection is platform-aware and deterministic so the same image
// always maps to the same digest across rebuilds: the available manifest
// matching the requested platform wins (satisfied); a lone available image
// manifest keeps its digest — single-platform images stay identifiable
// whatever their platform — without satisfying a different requested
// platform; otherwise fall back to inspect.ID, which never satisfies the
// request.
//
// Both the digest producers and the platform checks must go through this
// single selection so the digest recorded and the platform validated always
// refer to the same manifest.
func matchLocalManifest(inspect image.InspectResponse, platform platforms.Matcher) (string, bool) {
	var available []image.ManifestSummary
	for _, m := range inspect.Manifests {
		if m.Kind == image.ManifestKindImage && m.Available {
			available = append(available, m)
		}
	}
	for _, m := range available {
		if m.ImageData != nil && platform.Match(m.ImageData.Platform) {
			return m.ID, true
		}
	}
	if len(available) == 1 {
		m := available[0]
		if m.ImageData == nil {
			// engines may omit per-manifest image data (seen with locally
			// built, never-pushed images): fall back to the inspect's flat
			// platform fields, like the manifest-less path does, instead of
			// reporting the platform unsatisfied and triggering a pull of a
			// possibly local-only image
			return m.ID, platform.Match(specs.Platform{
				Architecture: inspect.Architecture,
				OS:           inspect.Os,
				Variant:      inspect.Variant,
			})
		}
		return m.ID, false
	}
	return inspect.ID, false
}

// inspectLocalContent inspects ref and returns its canonical content digest
// for the requested platform (empty means the host default), plus whether the
// local image satisfies that platform — see localContentDigest.
func (s *composeService) inspectLocalContent(ctx context.Context, ref string, platform string) (string, bool, error) {
	opts, err := s.imageInspectOptions(ctx)
	if err != nil {
		return "", false, err
	}
	inspected, err := s.apiClient().ImageInspect(ctx, ref, opts...)
	if err != nil {
		return "", false, err
	}
	return localContentDigest(inspected, platform)
}
