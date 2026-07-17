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

	"github.com/containerd/errdefs"
	"github.com/containerd/platforms"
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

func TestContentDigest(t *testing.T) {
	amd64 := platforms.Only(specs.Platform{OS: "linux", Architecture: "amd64"})
	arm64 := platforms.Only(specs.Platform{OS: "linux", Architecture: "arm64"})

	t.Run("no manifests falls back to the plain image ID", func(t *testing.T) {
		inspect := image.InspectResponse{ID: "sha256:top"}
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:top")
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
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:amd64")
	})

	t.Run("single image manifest is used even when the platform does not match", func(t *testing.T) {
		// single-platform image built for a non-host platform stays resolvable
		inspect := image.InspectResponse{
			ID:        "sha256:index",
			Manifests: []image.ManifestSummary{imageManifest("sha256:arm64", "arm64", true)},
		}
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:arm64")
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
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:amd64")
		assert.Equal(t, contentDigest(inspect, arm64), "sha256:arm64")
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
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:arm64")
	})

	t.Run("only attestation manifests falls back to the plain image ID", func(t *testing.T) {
		inspect := image.InspectResponse{
			ID:        "sha256:top",
			Manifests: []image.ManifestSummary{attestationManifest()},
		}
		assert.Equal(t, contentDigest(inspect, amd64), "sha256:top")
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
		assert.Equal(t, contentDigest(inspect, windows), "sha256:index")
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
	return api, tested.(*composeService)
}

func TestGetImageSummariesUsesContentDigest(t *testing.T) {
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

	summaries, err := tested.getImageSummaries(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, summaries["foo:1"].ID, "sha256:image")
}

func TestGetImageSummariesLegacyEngineUsesPlainID(t *testing.T) {
	// Engine < 28.0 (API < 1.48) can't report manifests, so we keep the plain ID.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.47")

	inspect := image.InspectResponse{ID: "sha256:plain"}
	api.EXPECT().
		ImageInspect(anyCancellableContext(), "foo:1").
		Return(client.ImageInspectResult{InspectResponse: inspect}, nil)

	summaries, err := tested.getImageSummaries(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, summaries["foo:1"].ID, "sha256:plain")
}

func TestGetImageSummariesSkipsMissingImages(t *testing.T) {
	// Registry-only images (push/multi-platform) aren't inspectable locally;
	// they must be omitted so the caller keeps the Bake-reported digest.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.48")

	api.EXPECT().
		ImageInspect(anyCancellableContext(), "missing:1", gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

	summaries, err := tested.getImageSummaries(t.Context(), []string{"missing:1"})
	assert.NilError(t, err)
	_, ok := summaries["missing:1"]
	assert.Assert(t, !ok)
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
