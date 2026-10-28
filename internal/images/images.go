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

// Package images implements the operations on the images of a project: listing
// the images its containers run, working out the identity of a local image, and
// removing the images `down` and `watch` leave behind.
package images

import (
	"context"
	"fmt"
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
	godigest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/concurrency"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
)

// Docker Engine API versions the operations of this package gate on.
const (
	// apiVersion148 is Docker Engine API 1.48 (Engine v28.0): image inspect can
	// return per-manifest data.
	apiVersion148 = "1.48"
	// apiVersion149 is Docker Engine API 1.49 (Engine v28.1): image inspect
	// accepts a platform.
	apiVersion149 = "1.49"
)

// Backend is what the operations need from the engine they act on.
type Backend interface {
	// APIClient talks to the Docker daemon.
	APIClient() client.APIClient
	// MaxConcurrency bounds the daemon calls made in parallel, 0 meaning no bound.
	MaxConcurrency() int
	// RuntimeAPIVersion is the API version negotiated with the daemon.
	RuntimeAPIVersion(ctx context.Context) (string, error)
	// ClassifyRemovalError reports whether the error of a resource removal is a
	// benign "still in use" or "already gone", shared with the removal of the
	// other resources of a project so the two agree on what is tolerable.
	ClassifyRemovalError(err error) (stillInUse, alreadyGone bool)
}

// Service runs the image operations on a Backend.
type Service struct {
	backend Backend
}

// New creates a Service acting on the given Backend.
func New(backend Backend) *Service {
	return &Service{backend: backend}
}

// Images lists the images the containers of a project run.
func (s *Service) Images(ctx context.Context, projectName string, options api.ImagesOptions) (map[string]api.ImageSummary, error) {
	projectName = strings.ToLower(projectName)
	allContainers, err := s.backend.APIClient().ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: naming.ProjectFilter(projectName),
	})
	if err != nil {
		return nil, err
	}
	var containers []container.Summary
	if len(options.Services) > 0 {
		containers = naming.InServices(allContainers.Items, options.Services)
	} else {
		containers = allContainers.Items
	}

	// The daemon validates the platform field in ImageInspect against the
	// negotiated API version from the request path, not the server's own max version.
	version, err := s.backend.RuntimeAPIVersion(ctx)
	if err != nil {
		return nil, err
	}
	withPlatform := versions.GreaterThanOrEqualTo(version, apiVersion149)

	summary := map[string]api.ImageSummary{}
	var mux sync.Mutex
	eg, ctx := concurrency.NewErrgroup(ctx, s.backend.MaxConcurrency())
	for _, ctr := range containers {
		eg.Go(func() error {
			img, err := s.containerImageSummary(ctx, ctr, withPlatform)
			if err != nil {
				return err
			}
			mux.Lock()
			defer mux.Unlock()
			summary[naming.CanonicalContainerName(ctr)] = img
			return nil
		})
	}

	err = eg.Wait()
	return summary, err
}

// containerImageSummary describes the image a container was created from. The
// image record may not exist anymore: under the containerd image store the
// daemon drops the dangling old index when a rebuild with identical content
// moves the tag — without compose recreating the container (see #13636) — and
// `docker rmi -f` may remove the image of a running container. In that case
// report what the container itself knows rather than failing the whole
// listing (#14014).
func (s *Service) containerImageSummary(ctx context.Context, c container.Summary, withPlatform bool) (api.ImageSummary, error) {
	var repository, tag string
	if _, err := godigest.Parse(c.Image); err != nil {
		// c.Image is a reference, not the raw image ID the engine reports
		// when the image record is gone or its tag was moved
		if ref, err := reference.ParseDockerRef(c.Image); err == nil {
			repository = reference.FamiliarName(ref)
			if tagged, ok := ref.(reference.Tagged); ok {
				tag = tagged.Tag()
			}
		}
	}

	img, err := s.backend.APIClient().ImageInspect(ctx, c.Image)
	if errdefs.IsNotFound(err) {
		return api.ImageSummary{
			ID:         c.ImageID,
			Repository: repository,
			Tag:        tag,
		}, nil
	}
	if err != nil {
		return api.ImageSummary{}, err
	}
	id := img.ID // platform-specific image ID can't be combined with image tag, see https://github.com/moby/moby/issues/49995

	if withPlatform && c.ImageManifestDescriptor != nil && c.ImageManifestDescriptor.Platform != nil {
		img, err = s.backend.APIClient().ImageInspect(ctx, c.Image, client.ImageInspectWithPlatform(c.ImageManifestDescriptor.Platform))
		if err != nil {
			return api.ImageSummary{}, err
		}
	}

	var created *time.Time
	if img.Created != "" {
		t, err := time.Parse(time.RFC3339Nano, img.Created)
		if err != nil {
			return api.ImageSummary{}, err
		}
		created = &t
	}

	return api.ImageSummary{
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
	}, nil
}

// InspectLocalImages inspects the given references in parallel, requesting
// per-manifest data on engines that support it. References not found locally
// are simply absent from the result.
func (s *Service) InspectLocalImages(ctx context.Context, repoTags []string) (map[string]client.ImageInspectResult, error) {
	opts, err := s.ImageInspectOptions(ctx)
	if err != nil {
		return nil, err
	}
	inspections := map[string]client.ImageInspectResult{}
	l := sync.Mutex{}
	eg, ctx := concurrency.NewErrgroup(ctx, s.backend.MaxConcurrency())
	for _, repoTag := range repoTags {
		eg.Go(func() error {
			inspect, err := s.backend.APIClient().ImageInspect(ctx, repoTag, opts...)
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

// ImageInspectOptions requests per-manifest data when the engine supports it
// (see manifestsSupported).
func (s *Service) ImageInspectOptions(ctx context.Context) ([]client.ImageInspectOption, error) {
	withManifests, err := s.manifestsSupported(ctx)
	if err != nil {
		return nil, err
	}
	if !withManifests {
		return nil, nil
	}
	return []client.ImageInspectOption{client.ImageInspectWithManifests(true)}, nil
}

func Summarize(repoTag string, inspect client.ImageInspectResult) api.ImageSummary {
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
	id, _, _ := LocalContentDigest(inspect, "")
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
func (s *Service) manifestsSupported(ctx context.Context) (bool, error) {
	version, err := s.backend.RuntimeAPIVersion(ctx)
	if err != nil {
		return false, err
	}
	return versions.GreaterThanOrEqualTo(version, apiVersion148), nil
}

// CanonicalBuiltDigest resolves the canonical content digest of a just-built
// image so the recorded identity matches what later runs compute for the same
// local image. Registry-only builds (push-only, multi-platform without load)
// are not locally inspectable and keep the builder-reported digest: a volatile
// but honest value — an actual rebuild is still detected — preferred over a
// stable marker that would hide real image changes. Best effort: never fails
// an already-successful build.
func (s *Service) CanonicalBuiltDigest(ctx context.Context, imageRef, platform, builderDigest string) string {
	id, _, err := s.InspectLocalContent(ctx, imageRef, platform)
	if err != nil {
		logrus.Debugf("unable to resolve content digest for built image %s, keeping builder-reported digest: %v", imageRef, err)
		return builderDigest
	}
	return id
}

// LocalContentDigest returns the digest identifying an inspected image's
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
//
// The converse also holds: this platform-specific digest is ONLY an identity
// for staleness comparison. Pinning image references in a reproducible
// compose model (publish / config --resolve-image-digests) must keep using
// the registry descriptor digest — the multi-platform index — through
// ImageDigestResolver, or the published file would be bound to the platform
// of whoever resolved it.
func LocalContentDigest(inspect client.ImageInspectResult, platform string) (string, bool, error) {
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

// InspectLocalContent inspects ref and returns its canonical content digest
// for the requested platform (empty means the host default), plus whether the
// local image satisfies that platform — see LocalContentDigest.
func (s *Service) InspectLocalContent(ctx context.Context, ref string, platform string) (string, bool, error) {
	opts, err := s.ImageInspectOptions(ctx)
	if err != nil {
		return "", false, err
	}
	inspected, err := s.backend.APIClient().ImageInspect(ctx, ref, opts...)
	if err != nil {
		return "", false, err
	}
	return LocalContentDigest(inspected, platform)
}
