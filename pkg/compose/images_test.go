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
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	args := projectFilter(strings.ToLower(testProject))
	listOpts := client.ContainerListOptions{All: true, Filters: args}
	api.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).Return(client.PingResult{APIVersion: "1.96"}, nil).AnyTimes()
	api.EXPECT().ClientVersion().Return("1.96").AnyTimes()
	timeStr1 := "2025-06-06T06:06:06.000000000Z"
	created1, _ := time.Parse(time.RFC3339Nano, timeStr1)
	timeStr2 := "2025-03-03T03:03:03.000000000Z"
	created2, _ := time.Parse(time.RFC3339Nano, timeStr2)
	image1 := imageInspect("image1", "foo:1", 12345, timeStr1)
	image2 := imageInspect("image2", "bar:2", 67890, timeStr2)
	api.EXPECT().ImageInspect(anyCancellableContext(), "foo:1").Return(client.ImageInspectResult{InspectResponse: image1}, nil).MaxTimes(2)
	api.EXPECT().ImageInspect(anyCancellableContext(), "bar:2").Return(client.ImageInspectResult{InspectResponse: image2}, nil)
	c1 := containerDetail("service1", "123", container.StateRunning, "foo:1")
	c2 := containerDetail("service1", "456", container.StateRunning, "bar:2")
	c2.Ports = []container.PortSummary{{PublicPort: 80, PrivatePort: 90, IP: netip.MustParseAddr("127.0.0.1")}}
	c3 := containerDetail("service2", "789", container.StateExited, "foo:1")
	api.EXPECT().ContainerList(t.Context(), listOpts).Return(client.ContainerListResult{
		Items: []container.Summary{c1, c2, c3},
	}, nil)

	images, err := tested.Images(t.Context(), strings.ToLower(testProject), compose.ImagesOptions{})

	expected := map[string]compose.ImageSummary{
		"123": {
			ID:         "image1",
			Repository: "foo",
			Tag:        "1",
			Size:       12345,
			Created:    &created1,
		},
		"456": {
			ID:         "image2",
			Repository: "bar",
			Tag:        "2",
			Size:       67890,
			Created:    &created2,
		},
		"789": {
			ID:         "image1",
			Repository: "foo",
			Tag:        "1",
			Size:       12345,
			Created:    &created1,
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, images, expected)
}

func imageInspect(id string, imageReference string, size int64, created string) image.InspectResponse {
	return image.InspectResponse{
		ID: id,
		RepoTags: []string{
			"someRepo:someTag",
			imageReference,
		},
		Size:    size,
		Created: created,
	}
}

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

func TestMatchLocalManifest(t *testing.T) {
	amd64 := platforms.Only(specs.Platform{OS: "linux", Architecture: "amd64"})
	arm64 := platforms.Only(specs.Platform{OS: "linux", Architecture: "arm64"})

	match := func(t *testing.T, inspect image.InspectResponse, platform platforms.Matcher, wantID string, wantOK bool) {
		t.Helper()
		id, ok := matchLocalManifest(inspect, platform)
		assert.Equal(t, id, wantID)
		assert.Equal(t, ok, wantOK)
	}

	t.Run("no manifests falls back to the plain image ID, unsatisfied", func(t *testing.T) {
		match(t, image.InspectResponse{ID: "sha256:top"}, amd64, "sha256:top", false)
	})

	t.Run("attested image ignores the attestation manifest", func(t *testing.T) {
		// sha256:index changes on every build due to attestation metadata churn;
		// the image-kind manifest digest is what actually reflects the content.
		inspect := image.InspectResponse{
			ID: "sha256:index",
			Manifests: []image.ManifestSummary{
				imageManifest("sha256:amd64", "amd64", true),
				attestationManifest(),
			},
		}
		match(t, inspect, amd64, "sha256:amd64", true)
	})

	t.Run("single image manifest keeps its digest but does not satisfy another platform", func(t *testing.T) {
		// single-platform image built for a non-host platform stays resolvable
		inspect := image.InspectResponse{
			ID:        "sha256:index",
			Manifests: []image.ManifestSummary{imageManifest("sha256:arm64", "arm64", true)},
		}
		match(t, inspect, amd64, "sha256:arm64", false)
		match(t, inspect, arm64, "sha256:arm64", true)
	})

	t.Run("multi-platform picks the matching platform manifest", func(t *testing.T) {
		inspect := image.InspectResponse{
			ID: "sha256:index",
			Manifests: []image.ManifestSummary{
				imageManifest("sha256:amd64", "amd64", true),
				imageManifest("sha256:arm64", "arm64", true),
				attestationManifest(),
			},
		}
		match(t, inspect, amd64, "sha256:amd64", true)
		match(t, inspect, arm64, "sha256:arm64", true)
	})

	t.Run("unavailable manifests are skipped", func(t *testing.T) {
		// amd64 is referenced by the index but not pulled locally; only arm64 is usable
		inspect := image.InspectResponse{
			ID: "sha256:index",
			Manifests: []image.ManifestSummary{
				imageManifest("sha256:amd64", "amd64", false),
				imageManifest("sha256:arm64", "arm64", true),
			},
		}
		match(t, inspect, amd64, "sha256:arm64", false)
	})

	t.Run("only attestation manifests falls back to the plain image ID", func(t *testing.T) {
		inspect := image.InspectResponse{
			ID:        "sha256:top",
			Manifests: []image.ManifestSummary{attestationManifest()},
		}
		match(t, inspect, amd64, "sha256:top", false)
	})

	t.Run("ambiguous multi-platform with no match falls back to the plain image ID", func(t *testing.T) {
		inspect := image.InspectResponse{
			ID: "sha256:index",
			Manifests: []image.ManifestSummary{
				imageManifest("sha256:amd64", "amd64", true),
				imageManifest("sha256:arm64", "arm64", true),
			},
		}
		windows := platforms.Only(specs.Platform{OS: "windows", Architecture: "amd64"})
		match(t, inspect, windows, "sha256:index", false)
	})
}

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

func TestInspectLocalImagesUsesContentDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.48")

	inspect := image.InspectResponse{
		ID: "sha256:index", // attested index digest, churns every build
		Manifests: []image.ManifestSummary{
			imageManifest("sha256:image", "amd64", true),
			attestationManifest(),
		},
	}
	api.EXPECT().
		ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
		Return(client.ImageInspectResult{InspectResponse: inspect}, nil)

	inspections, err := tested.inspectLocalImages(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, imageSummary("foo:1", inspections["foo:1"]).ID, "sha256:image")
}

func TestInspectLocalImagesLegacyEngineUsesPlainID(t *testing.T) {
	// Engine < 28.0 (API < 1.48) can't report manifests, so we keep the plain ID.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.47")

	inspect := image.InspectResponse{ID: "sha256:plain"}
	api.EXPECT().
		ImageInspect(anyCancellableContext(), "foo:1").
		Return(client.ImageInspectResult{InspectResponse: inspect}, nil)

	inspections, err := tested.inspectLocalImages(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, imageSummary("foo:1", inspections["foo:1"]).ID, "sha256:plain")
}

func TestInspectLocalImagesSkipsMissingImages(t *testing.T) {
	// Registry-only images (push/multi-platform) aren't inspectable locally;
	// they must be omitted so the caller keeps the Bake-reported digest.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.48")

	api.EXPECT().
		ImageInspect(anyCancellableContext(), "missing:1", gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

	inspections, err := tested.inspectLocalImages(t.Context(), []string{"missing:1"})
	assert.NilError(t, err)
	_, ok := inspections["missing:1"]
	assert.Assert(t, !ok)
}

func TestInspectLocalContent(t *testing.T) {
	t.Run("manifests path selects the pinned platform", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:index",
				Manifests: []image.ManifestSummary{
					imageManifest("sha256:amd64", "amd64", true),
					imageManifest("sha256:arm64", "arm64", true),
				},
			}}, nil)

		id, ok, err := tested.inspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Equal(t, id, "sha256:arm64")
		assert.Assert(t, ok)
	})

	t.Run("manifests path reports an unavailable pinned platform", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID:        "sha256:index",
				Manifests: []image.ManifestSummary{imageManifest("sha256:amd64", "amd64", true)},
			}}, nil)

		_, ok, err := tested.inspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Assert(t, !ok)
	})

	t.Run("legacy engine falls back to flat platform fields", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.47")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1").
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:plain", Os: "linux", Architecture: "amd64",
			}}, nil).Times(2)

		id, ok, err := tested.inspectLocalContent(t.Context(), "foo:1", "linux/amd64")
		assert.NilError(t, err)
		assert.Equal(t, id, "sha256:plain")
		assert.Assert(t, ok)

		_, ok, err = tested.inspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Assert(t, !ok)
	})
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
					Name:         "app",
					Image:        "foo:1",
					Platform:     "linux/s390x",
					CustomLabels: types.Labels{},
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
		assert.NilError(t, tested.ensureImagesExists(t.Context(), project, nil, true))
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
	pinnedService := types.ServiceConfig{Name: "app", Platform: "linux/s390x"}

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

func TestCanonicalBuiltDigest(t *testing.T) {
	t.Run("locally inspectable build resolves to the content digest", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "built:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:index",
				Manifests: []image.ManifestSummary{
					imageManifest("sha256:image", "amd64", true),
					attestationManifest(),
				},
			}}, nil)

		got := tested.canonicalBuiltDigest(t.Context(), "built:1", "", "sha256:bakeindex")
		assert.Equal(t, got, "sha256:image")
	})

	t.Run("registry-only build keeps the builder-reported digest", func(t *testing.T) {
		// push-only / multi-platform-only builds never land in the local
		// store: keep the builder digest — volatile but honest (a real
		// rebuild is detected) rather than a stable marker hiding changes.
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestComposeService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "pushed:1", gomock.Any()).
			Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

		got := tested.canonicalBuiltDigest(t.Context(), "pushed:1", "", "sha256:bakeindex")
		assert.Equal(t, got, "sha256:bakeindex")
	})
}

func containerDetail(service string, id string, status container.ContainerState, imageName string) container.Summary {
	return container.Summary{
		ID:     id,
		Names:  []string{"/" + id},
		Image:  imageName,
		Labels: containerLabels(service, false),
		State:  status,
	}
}
