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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"golang.org/x/sync/semaphore"

	"github.com/docker/compose/v5/internal/images"
	"github.com/docker/compose/v5/pkg/api"
)

// ImagePruneMode aliases the pkg/api definition: the legal values of
// DownOptions.Images belong to the SDK surface, kept aliased here for the
// existing consumers of this package.
type ImagePruneMode = api.ImagePruneMode

const (
	// ImagePruneNone indicates that no project images should be removed.
	ImagePruneNone = api.ImagePruneNone
	// ImagePruneLocal indicates that only images built locally by Compose
	// should be removed.
	ImagePruneLocal = api.ImagePruneLocal
	// ImagePruneAll indicates that all project-associated images, including
	// remote images should be removed.
	ImagePruneAll = api.ImagePruneAll
)

// The pruning of images lives in internal/images. ImagePruneOptions and
// ImagePruner stay exported from this package as aliases, so the SDK surface
// does not change.

// ImagePruneOptions controls the behavior of image pruning.
type ImagePruneOptions = images.PruneOptions

// ImagePruner handles image removal during Compose `down` operations.
type ImagePruner = images.Pruner

// NewImagePruner creates an ImagePruner object for a project. limiter may be
// nil, meaning unbounded.
func NewImagePruner(imageClient client.ImageAPIClient, project *types.Project, limiter *semaphore.Weighted) *ImagePruner {
	return images.NewPruner(imageClient, project, limiter)
}

func imageBelongsToKnownService(project *types.Project, removeOrphans bool, img image.Summary) bool {
	return images.ImageBelongsToKnownService(project, removeOrphans, img)
}

func (s *composeService) removeImages(ctx context.Context, imgs []image.Summary, limiter *semaphore.Weighted) (removed, stillInUse []string, err error) {
	return s.images().RemoveImages(ctx, imgs, limiter)
}

func (s *composeService) removeDanglingImages(ctx context.Context, projectName string, keep func(image.Summary) bool) ([]string, error) {
	return s.images().RemoveDanglingImages(ctx, projectName, keep)
}

func (s *composeService) eligibleDanglingImages(ctx context.Context, projectName string, keep func(image.Summary) bool) ([]image.Summary, error) {
	return s.images().EligibleDanglingImages(ctx, projectName, keep)
}
