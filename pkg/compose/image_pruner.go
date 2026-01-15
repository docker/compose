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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose/v5/pkg/api"
)

type ImagePruneMode string

const (
	ImagePruneNone  ImagePruneMode = ""
	ImagePruneLocal ImagePruneMode = "local"
	ImagePruneAll   ImagePruneMode = "all"
)

type ImagePruneOptions struct {
	Mode          ImagePruneMode
	RemoveOrphans bool
}

type ImagePruner struct {
	client  client.APIClient
	project *types.Project
}

func NewImagePruner(apiClient client.APIClient, project *types.Project) *ImagePruner {
	return &ImagePruner{
		client:  apiClient,
		project: project,
	}
}

func (p *ImagePruner) ImagesToPrune(ctx context.Context, opts ImagePruneOptions) ([]string, error) {
	if opts.Mode == ImagePruneNone {
		return nil, nil
	}
	if opts.Mode != ImagePruneLocal && opts.Mode != ImagePruneAll {
		return nil, fmt.Errorf("unsupported image prune mode: %s", opts.Mode)
	}

	var (
		imagesToCheck []string // subject to container usage check
		imagesFinal   []string // always removed
	)

	// --rmi=all
	if opts.Mode == ImagePruneAll {
		named, err := p.namedImages(ctx)
		if err != nil {
			return nil, err
		}
		imagesToCheck = append(imagesToCheck, named...)
	}

	projectImages, err := p.labeledLocalImages(ctx)
	if err != nil {
		return nil, err
	}

	for _, img := range projectImages {
		if len(img.RepoTags) == 0 {
			continue
		}

		var shouldPrune bool
		if opts.RemoveOrphans {
			shouldPrune = true
		} else {
			if _, err := p.project.GetService(img.Labels[api.ServiceLabel]); err == nil {
				shouldPrune = true
			}
		}

		if shouldPrune {
			imagesToCheck = append(imagesToCheck, img.RepoTags[0])
		}
	}

	// legacy / no-label images — ALWAYS removed, no container check
	fallbackImages, err := p.unlabeledLocalImages(ctx)
	if err != nil {
		return nil, err
	}
	imagesFinal = append(imagesFinal, fallbackImages...)

	// only check containers if needed
	if len(imagesToCheck) > 0 {
		inUse, err := p.imagesInUse(ctx)
		if err != nil {
			return nil, err
		}

		for _, img := range imagesToCheck {
			if _, used := inUse[img]; used {
				continue
			}
			imagesFinal = append(imagesFinal, img)
		}
	}

	return normalizeAndDedupeImages(imagesFinal), nil
}

func (p *ImagePruner) imagesInUse(ctx context.Context) (map[string]struct{}, error) {
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			projectFilter(p.project.Name),
			filters.Arg("label", api.OneoffLabel+"=False"),
			filters.Arg("label", api.ConfigHashLabel),
		),
	}

	containers, err := p.client.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}

	inUse := make(map[string]struct{})
	for _, c := range containers {
		if c.Image != "" {
			inUse[c.Image] = struct{}{}
		}
	}
	return inUse, nil
}

func (p *ImagePruner) namedImages(ctx context.Context) ([]string, error) {
	var images []string
	for _, service := range p.project.Services {
		if service.Image != "" {
			images = append(images, service.Image)
		}
	}
	return p.filterImagesByExistence(ctx, images)
}

func (p *ImagePruner) labeledLocalImages(ctx context.Context) ([]image.Summary, error) {
	opts := image.ListOptions{
		Filters: filters.NewArgs(
			projectFilter(p.project.Name),
			filters.Arg("dangling", "false"),
		),
	}
	return p.client.ImageList(ctx, opts)
}

func (p *ImagePruner) unlabeledLocalImages(ctx context.Context) ([]string, error) {
	var images []string
	for _, service := range p.project.Services {
		if service.Image == "" {
			images = append(images, api.GetImageNameOrDefault(service, p.project.Name))
		}
	}
	return p.filterImagesByExistence(ctx, images)
}

func (p *ImagePruner) filterImagesByExistence(ctx context.Context, imageNames []string) ([]string, error) {
	var mu sync.Mutex
	var ret []string

	eg, ctx := errgroup.WithContext(ctx)
	for _, img := range imageNames {
		img := img
		eg.Go(func() error {
			_, err := p.client.ImageInspect(ctx, img)
			if errdefs.IsNotFound(err) {
				return nil
			}
			mu.Lock()
			ret = append(ret, img)
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

func normalizeAndDedupeImages(images []string) []string {
	seen := make(map[string]struct{})
	for _, img := range images {
		ref, err := reference.ParseNormalizedNamed(img)
		if err == nil {
			ref = reference.TagNameOnly(ref)
			img = reference.FamiliarString(ref)
		}
		seen[img] = struct{}{}
	}

	ret := make([]string, 0, len(seen))
	for img := range seen {
		ret = append(ret, img)
	}
	sort.Strings(ret)
	return ret
}
