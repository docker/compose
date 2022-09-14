/*
   Copyright 2022 Docker Compose CLI authors

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
	"sort"
	"sync"

	"github.com/compose-spec/compose-go/types"
	"github.com/distribution/distribution/v3/reference"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v2/pkg/api"
)

// ImagePruneMode controls how aggressively images associated with the project
// are removed from the engine.
type ImagePruneMode string

const (
	// ImagePruneNone indicates that no project images should be removed.
	ImagePruneNone ImagePruneMode = ""
	// ImagePruneLocal indicates that only images built locally by Compose
	// should be removed.
	ImagePruneLocal ImagePruneMode = "local"
	// ImagePruneAll indicates that all project-associated images, including
	// remote images should be removed.
	ImagePruneAll ImagePruneMode = "all"
)

// ImagePruneOptions controls the behavior of image pruning.
type ImagePruneOptions struct {
	Mode ImagePruneMode

	// RemoveOrphans will result in the removal of images that were built for
	// the project regardless of whether they are for a known service if true.
	RemoveOrphans bool
}

// ImagePruner handles image removal during Compose `down` operations.
type ImagePruner struct {
	client  client.ImageAPIClient
	project *types.Project
}

// NewImagePruner creates an ImagePruner object for a project.
func NewImagePruner(imageClient client.ImageAPIClient, project *types.Project) *ImagePruner {
	return &ImagePruner{
		client:  imageClient,
		project: project,
	}
}

// ImagesToPrune returns the set of images that should be removed.
func (p *ImagePruner) ImagesToPrune(ctx context.Context, opts ImagePruneOptions) ([]string, error) {
	if opts.Mode == ImagePruneNone {
		return nil, nil
	} else if opts.Mode != ImagePruneLocal && opts.Mode != ImagePruneAll {
		return nil, fmt.Errorf("unsupported image prune mode: %s", opts.Mode)
	}
	var images []string

	if opts.Mode == ImagePruneAll {
		namedImages, err := p.namedImages(ctx)
		if err != nil {
			return nil, err
		}
		images = append(images, namedImages...)
	}

	projectImages, err := p.labeledLocalImages(ctx)
	if err != nil {
		return nil, err
	}
	for _, img := range projectImages {
		if len(img.RepoTags) == 0 {
			// currently, we're only pruning the tagged references, but
			// if we start removing the dangling images and grouping by
			// service, we can remove this (and should rely on `Image::ID`)
			continue
		}

		var shouldPrune bool
		if opts.RemoveOrphans {
			// indiscriminately prune all project images even if they're not
			// referenced by the current Compose state (e.g. the service was
			// removed from YAML)
			shouldPrune = true
		} else {
			// only prune the image if it belongs to a known service for the
			// project AND is either an implicitly-named, locally-built image
			// or `--rmi=all` has been specified.
			// TODO(milas): now that Compose labels the images it builds, this
			// makes less sense; arguably, locally-built but explicitly-named
			// images should be removed with `--rmi=local` as well.
			service, err := p.project.GetService(img.Labels[api.ServiceLabel])
			if err == nil && (opts.Mode == ImagePruneAll || service.Image == "") {
				shouldPrune = true
			}
		}

		if shouldPrune {
			images = append(images, img.RepoTags[0])
		}
	}

	fallbackImages, err := p.unlabeledLocalImages(ctx)
	if err != nil {
		return nil, err
	}
	images = append(images, fallbackImages...)

	images = normalizeAndDedupeImages(images)
	return images, nil
}

// namedImages are those that are explicitly named in the service config.
//
// These could be registry-only images (no local build), hybrid (support build
// as a fallback if cannot pull), or local-only (image does not exist in a
// registry).
func (p *ImagePruner) namedImages(ctx context.Context) ([]string, error) {
	var images []string
	for _, service := range p.project.Services {
		if service.Image == "" {
			continue
		}
		images = append(images, service.Image)
	}
	return p.filterImagesByExistence(ctx, images)
}

// labeledLocalImages are images that were locally-built by a current version of
// Compose (it did not always label built images).
//
// The image name could either have been defined by the user or implicitly
// created from the project + service name.
func (p *ImagePruner) labeledLocalImages(ctx context.Context) ([]moby.ImageSummary, error) {
	imageListOpts := moby.ImageListOptions{
		Filters: filters.NewArgs(
			projectFilter(p.project.Name),
			// TODO(milas): we should really clean up the dangling images as
			// well (historically we have NOT); need to refactor this to handle
			// it gracefully without producing confusing CLI output, i.e. we
			// do not want to print out a bunch of untagged/dangling image IDs,
			// they should be grouped into a logical operation for the relevant
			// service
			filters.Arg("dangling", "false"),
		),
	}
	projectImages, err := p.client.ImageList(ctx, imageListOpts)
	if err != nil {
		return nil, err
	}
	return projectImages, nil
}

// unlabeledLocalImages are images that match the implicit naming convention
// for locally-built images but did not get labeled, presumably because they
// were produced by an older version of Compose.
//
// This is transitional to ensure `down` continues to work as expected on
// projects built/launched by previous versions of Compose. It can safely
// be removed after some time.
func (p *ImagePruner) unlabeledLocalImages(ctx context.Context) ([]string, error) {
	var images []string
	for _, service := range p.project.Services {
		if service.Image != "" {
			continue
		}
		img := api.GetImageNameOrDefault(service, p.project.Name)
		images = append(images, img)
	}
	return p.filterImagesByExistence(ctx, images)
}

// filterImagesByExistence returns the subset of images that exist in the
// engine store.
//
// NOTE: Any transient errors communicating with the API will result in an
// image being returned as "existing", as this method is exclusively used to
// find images to remove, so the worst case of being conservative here is an
// attempt to remove an image that doesn't exist, which will cause a warning
// but is otherwise harmless.
func (p *ImagePruner) filterImagesByExistence(ctx context.Context, imageNames []string) ([]string, error) {
	var mu sync.Mutex
	var ret []string

	eg, ctx := errgroup.WithContext(ctx)
	for _, img := range imageNames {
		img := img
		eg.Go(func() error {
			_, _, err := p.client.ImageInspectWithRaw(ctx, img)
			if errdefs.IsNotFound(err) {
				// err on the side of caution: only skip if we successfully
				// queried the API and got back a definitive "not exists"
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			ret = append(ret, img)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return ret, nil
}

// normalizeAndDedupeImages returns the unique set of images after normalization.
func normalizeAndDedupeImages(images []string) []string {
	seen := make(map[string]struct{}, len(images))
	for _, img := range images {
		// since some references come from user input (service.image) and some
		// come from the engine API, we standardize them, opting for the
		// familiar name format since they'll also be displayed in the CLI
		ref, err := reference.ParseNormalizedNamed(img)
		if err == nil {
			ref = reference.TagNameOnly(ref)
			img = reference.FamiliarString(ref)
		}
		seen[img] = struct{}{}
	}
	ret := make([]string, 0, len(seen))
	for v := range seen {
		ret = append(ret, v)
	}
	// ensure a deterministic return result - the actual ordering is not useful
	sort.Strings(ret)
	return ret
}
