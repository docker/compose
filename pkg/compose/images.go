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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/images"
	"github.com/docker/compose/v5/pkg/api"
)

// The images command, the identity of a local image and the removal of images
// live in internal/images: composeService serves them by handing that package
// the engine they act on. What follows are the forwarders the rest of this
// package still calls, and serviceImageDigest, which depends on the
// pinnedImageDigest type of build.go.

func (s *composeService) images() *images.Service {
	return images.New(imagesBackend{s})
}

func (s *composeService) Images(ctx context.Context, projectName string, options api.ImagesOptions) (map[string]api.ImageSummary, error) {
	return s.images().Images(ctx, projectName, options)
}

func (s *composeService) inspectLocalImages(ctx context.Context, repoTags []string) (map[string]client.ImageInspectResult, error) {
	return s.images().InspectLocalImages(ctx, repoTags)
}

func (s *composeService) imageInspectOptions(ctx context.Context) ([]client.ImageInspectOption, error) {
	return s.images().ImageInspectOptions(ctx)
}

func imageSummary(repoTag string, inspect client.ImageInspectResult) api.ImageSummary {
	return images.Summarize(repoTag, inspect)
}

func (s *composeService) canonicalBuiltDigest(ctx context.Context, imageRef, platform, builderDigest string) string {
	return s.images().CanonicalBuiltDigest(ctx, imageRef, platform, builderDigest)
}

func localContentDigest(inspect client.ImageInspectResult, platform string) (string, bool, error) {
	return images.LocalContentDigest(inspect, platform)
}

func (s *composeService) inspectLocalContent(ctx context.Context, ref string, platform string) (string, bool, error) {
	return s.images().InspectLocalContent(ctx, ref, platform)
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

// imagesBackend is the engine of a composeService, as internal/images sees it.
type imagesBackend struct {
	s *composeService
}

func (b imagesBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b imagesBackend) MaxConcurrency() int {
	return b.s.maxConcurrency
}

func (b imagesBackend) RuntimeAPIVersion(ctx context.Context) (string, error) {
	return b.s.RuntimeAPIVersion(ctx)
}

func (b imagesBackend) ClassifyRemovalError(err error) (stillInUse, alreadyGone bool) {
	return classifyRemovalError(err)
}
