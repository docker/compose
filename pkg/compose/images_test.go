/*
   Copyright 2024 Docker Compose CLI authors

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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

//nolint:unparam // the tests of the engines without manifests moved to internal/images
func imageManifest(id, arch string, available bool) image.ManifestSummary {
	return image.ManifestSummary{
		ID:        id,
		Kind:      image.ManifestKindImage,
		Available: available,
		ImageData: &image.ImageProperties{
			Platform: specs.Platform{OS: "linux", Architecture: arch},
		},
	}
}

func attestationManifest() image.ManifestSummary {
	return image.ManifestSummary{ID: "sha256:att", Kind: image.ManifestKindAttestation, Available: true}
}

//nolint:unparam // the tests of older engines moved to internal/images
func newTestComposeService(t *testing.T, mockCtrl *gomock.Controller, apiVersion string) (*mocks.MockAPIClient, *composeService) {
	t.Helper()
	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	api.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: apiVersion}, nil).AnyTimes()
	api.EXPECT().ClientVersion().Return(apiVersion).AnyTimes()
	cli.EXPECT().ConfigFile().Return(configfile.New("")).AnyTimes()
	return api, tested.(*composeService)
}

// TestPlatformPinnedDigest covers two historic defects around
// `platform:`-pinned services:
//   - the com.docker.compose.image label must hold the digest of the PINNED
//     platform manifest, not the host's — all the way through
//     ensureImagesExists, whose final loop is the label's single writer;
//   - when the local image cannot satisfy the pinned platform, no label at all
//     must be written (the summary was just discarded as "wrong platform").
func TestPlatformPinnedDigest(t *testing.T) {
	// platforms are synthetic so neither can match the machine running the
	// tests: the host-side summary must fall back to the index digest while
	// the pinned resolution picks the service's platform manifest
	multiPlatform := image.InspectResponse{
		ID: "sha256:index",
		Manifests: []image.ManifestSummary{
			imageManifest("sha256:s390x", "s390x", true),
			imageManifest("sha256:riscv64", "riscv64", true),
		},
	}

	newProject := func() *types.Project {
		return &types.Project{
			Name: "p",
			Services: types.Services{
				"app": {
					Name: "app", ContainerSpec: types.ContainerSpec{
						Image:        "foo:1",
						Platform:     "linux/s390x",
						CustomLabels: types.Labels{},
					},
				},
			},
		}
	}

	t.Run("pinned platform digest is resolved from the shared inspect", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: multiPlatform}, nil) // a single inspect serves both the summary and the platform check

		project := newProject()
		imgs, pinned, err := tested.getLocalImagesDigests(t.Context(), project)
		assert.NilError(t, err)
		assert.Equal(t, imgs["foo:1"].ID, "sha256:index", "shared summary stays host-resolved")
		assert.Equal(t, pinned["app"], pinnedImageDigest{digest: "sha256:s390x", from: "sha256:index"})
	})

	t.Run("pinned platform digest lands in the label through ensureImagesExists", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: multiPlatform}, nil)

		project := newProject()
		assert.NilError(t, tested.ensureImagesExists(t.Context(), project, nil, true, false))
		assert.Equal(t, project.Services["app"].CustomLabels[compose.ImageDigestLabel], "sha256:s390x")
	})

	t.Run("platform mismatch discards the image and writes no label", func(t *testing.T) {
		amd64Only := image.InspectResponse{
			ID:        "sha256:index",
			Manifests: []image.ManifestSummary{imageManifest("sha256:riscv64", "riscv64", true)},
		}
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: amd64Only}, nil)

		project := newProject()
		imgs, pinned, err := tested.getLocalImagesDigests(t.Context(), project)
		assert.NilError(t, err)
		_, present := imgs["foo:1"]
		assert.Assert(t, !present)
		assert.Equal(t, len(pinned), 0)
		_, labelled := project.Services["app"].CustomLabels[compose.ImageDigestLabel]
		assert.Assert(t, !labelled)
	})
}

// TestServiceImageDigest covers the label-digest decision for platform-pinned
// services, notably when the shared summary entry was refreshed by a pull or
// build during the run: the refreshed digest was resolved for the platform of
// whichever service triggered it, so services sharing the image with another
// pinned platform must re-resolve theirs.
func TestServiceImageDigest(t *testing.T) {
	pinnedService := types.ServiceConfig{Name: "app", ContainerSpec: types.ContainerSpec{Platform: "linux/s390x"}}

	t.Run("unpinned service uses the shared digest, no inspect", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		_, tested := newTestComposeService(t, mockCtrl, "1.48")

		got := tested.serviceImageDigest(t.Context(), types.ServiceConfig{Name: "app"}, "foo:1",
			compose.ImageSummary{ID: "sha256:shared"}, nil)
		assert.Equal(t, got, "sha256:shared")
	})

	t.Run("valid pinned resolution is used without any inspect", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		_, tested := newTestComposeService(t, mockCtrl, "1.48")

		got := tested.serviceImageDigest(t.Context(), pinnedService, "foo:1",
			compose.ImageSummary{ID: "sha256:index"},
			map[string]pinnedImageDigest{"app": {digest: "sha256:s390x", from: "sha256:index"}})
		assert.Equal(t, got, "sha256:s390x")
	})

	t.Run("refreshed entry re-resolves the pinned platform", func(t *testing.T) {
		// the image was pulled/built during the run for ANOTHER service's
		// platform: the stale pre-pull resolution must not be used, and the
		// shared digest is not this service's platform either
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:refreshed",
				Manifests: []image.ManifestSummary{
					imageManifest("sha256:riscv64", "riscv64", true),
					imageManifest("sha256:s390x", "s390x", true),
				},
			}}, nil)

		got := tested.serviceImageDigest(t.Context(), pinnedService, "foo:1",
			compose.ImageSummary{ID: "sha256:refreshed"},
			map[string]pinnedImageDigest{"app": {digest: "sha256:stale", from: "sha256:index"}})
		assert.Equal(t, got, "sha256:s390x")
	})

	t.Run("unsatisfied pinned platform falls back to the shared digest", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID:        "sha256:refreshed",
				Manifests: []image.ManifestSummary{imageManifest("sha256:riscv64", "riscv64", true)},
			}}, nil)

		got := tested.serviceImageDigest(t.Context(), pinnedService, "foo:1",
			compose.ImageSummary{ID: "sha256:refreshed"}, nil)
		assert.Equal(t, got, "sha256:refreshed")
	})
}
