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

package images

import (
	"context"
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

	"github.com/docker/compose/v5/internal/naming"
	compose "github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

const testProject = "testProject"

// fakeBackend serves the API client it is given, and the API version a daemon
// of that version would have negotiated.
type fakeBackend struct {
	apiClient client.APIClient
	version   string
}

func (f *fakeBackend) APIClient() client.APIClient { return f.apiClient }
func (f *fakeBackend) MaxConcurrency() int         { return 0 }

func (f *fakeBackend) RuntimeAPIVersion(context.Context) (string, error) { return f.version, nil }

func (f *fakeBackend) ClassifyRemovalError(err error) (stillInUse, alreadyGone bool) {
	return errdefs.IsConflict(err), errdefs.IsNotFound(err)
}

func newTestService(t *testing.T, mockCtrl *gomock.Controller, apiVersion string) (*mocks.MockAPIClient, *Service) {
	t.Helper()
	api := mocks.NewMockAPIClient(mockCtrl)
	return api, New(&fakeBackend{apiClient: api, version: apiVersion})
}

func anyCancellableContext() gomock.Matcher {
	//nolint:forbidigo // This creates a context type for gomock matching, not for actual test usage
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	cancel()
	return gomock.AssignableToTypeOf(ctxWithCancel)
}

func TestImages(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api := mocks.NewMockAPIClient(mockCtrl)
	tested := New(&fakeBackend{apiClient: api, version: "1.96"})

	args := naming.ProjectFilter(strings.ToLower(testProject))
	listOpts := client.ContainerListOptions{All: true, Filters: args}
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

func TestImagesMissingImageRecord(t *testing.T) {
	// A container may reference an image record that no longer exists: under
	// the containerd image store, a rebuild with identical content moves the
	// tag and the daemon drops the old index the running container was created
	// from (https://github.com/docker/compose/issues/14014), and `docker rmi
	// -f` may remove the image of a running container. The listing must
	// degrade to what the container itself knows instead of failing.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api := mocks.NewMockAPIClient(mockCtrl)
	tested := New(&fakeBackend{apiClient: api, version: "1.96"})

	args := naming.ProjectFilter(strings.ToLower(testProject))
	listOpts := client.ContainerListOptions{All: true, Filters: args}

	// the raw image ID the engine reports once the image record is gone
	const goneID = "sha256:52c159e21b703f2c750d0bfd8d0af93324b46c0df0859331fb9489b7ec033d0c"
	gone := containerDetail("service1", "123", container.StateRunning, goneID)
	gone.ImageID = goneID
	api.EXPECT().ImageInspect(anyCancellableContext(), goneID).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)
	api.EXPECT().ContainerList(t.Context(), listOpts).Return(client.ContainerListResult{
		Items: []container.Summary{gone},
	}, nil)

	images, err := tested.Images(t.Context(), strings.ToLower(testProject), compose.ImagesOptions{})
	assert.NilError(t, err)
	assert.DeepEqual(t, images, map[string]compose.ImageSummary{
		"123": {ID: goneID},
	})
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

func TestInspectLocalImagesUsesContentDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestService(t, mockCtrl, "1.48")

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

	inspections, err := tested.InspectLocalImages(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, Summarize("foo:1", inspections["foo:1"]).ID, "sha256:image")
}

func TestInspectLocalImagesLegacyEngineUsesPlainID(t *testing.T) {
	// Engine < 28.0 (API < 1.48) can't report manifests, so we keep the plain ID.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestService(t, mockCtrl, "1.47")

	inspect := image.InspectResponse{ID: "sha256:plain"}
	api.EXPECT().
		ImageInspect(anyCancellableContext(), "foo:1").
		Return(client.ImageInspectResult{InspectResponse: inspect}, nil)

	inspections, err := tested.InspectLocalImages(t.Context(), []string{"foo:1"})
	assert.NilError(t, err)
	assert.Equal(t, Summarize("foo:1", inspections["foo:1"]).ID, "sha256:plain")
}

func TestInspectLocalImagesSkipsMissingImages(t *testing.T) {
	// Registry-only images (push/multi-platform) aren't inspectable locally;
	// they must be omitted so the caller keeps the Bake-reported digest.
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestService(t, mockCtrl, "1.48")

	api.EXPECT().
		ImageInspect(anyCancellableContext(), "missing:1", gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

	inspections, err := tested.InspectLocalImages(t.Context(), []string{"missing:1"})
	assert.NilError(t, err)
	_, ok := inspections["missing:1"]
	assert.Assert(t, !ok)
}

func TestInspectLocalContent(t *testing.T) {
	t.Run("manifests path selects the pinned platform", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:index",
				Manifests: []image.ManifestSummary{
					imageManifest("sha256:amd64", "amd64", true),
					imageManifest("sha256:arm64", "arm64", true),
				},
			}}, nil)

		id, ok, err := tested.InspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Equal(t, id, "sha256:arm64")
		assert.Assert(t, ok)
	})

	t.Run("manifests path reports an unavailable pinned platform", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID:        "sha256:index",
				Manifests: []image.ManifestSummary{imageManifest("sha256:amd64", "amd64", true)},
			}}, nil)

		_, ok, err := tested.InspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Assert(t, !ok)
	})

	t.Run("legacy engine falls back to flat platform fields", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestService(t, mockCtrl, "1.47")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "foo:1").
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:plain", Os: "linux", Architecture: "amd64",
			}}, nil).Times(2)

		id, ok, err := tested.InspectLocalContent(t.Context(), "foo:1", "linux/amd64")
		assert.NilError(t, err)
		assert.Equal(t, id, "sha256:plain")
		assert.Assert(t, ok)

		_, ok, err = tested.InspectLocalContent(t.Context(), "foo:1", "linux/arm64")
		assert.NilError(t, err)
		assert.Assert(t, !ok)
	})
}

func TestCanonicalBuiltDigest(t *testing.T) {
	t.Run("locally inspectable build resolves to the content digest", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "built:1", gomock.Any()).
			Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
				ID: "sha256:index",
				Manifests: []image.ManifestSummary{
					imageManifest("sha256:image", "amd64", true),
					attestationManifest(),
				},
			}}, nil)

		got := tested.CanonicalBuiltDigest(t.Context(), "built:1", "", "sha256:bakeindex")
		assert.Equal(t, got, "sha256:image")
	})

	t.Run("registry-only build keeps the builder-reported digest", func(t *testing.T) {
		// push-only / multi-platform-only builds never land in the local
		// store: keep the builder digest — volatile but honest (a real
		// rebuild is detected) rather than a stable marker hiding changes.
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		api, tested := newTestService(t, mockCtrl, "1.48")
		api.EXPECT().
			ImageInspect(anyCancellableContext(), "pushed:1", gomock.Any()).
			Return(client.ImageInspectResult{}, errdefs.ErrNotFound)

		got := tested.CanonicalBuiltDigest(t.Context(), "pushed:1", "", "sha256:bakeindex")
		assert.Equal(t, got, "sha256:bakeindex")
	})
}

func containerDetail(service string, id string, status container.ContainerState, imageName string) container.Summary {
	return container.Summary{
		ID:    id,
		Names: []string{"/" + id},
		Image: imageName,
		Labels: map[string]string{
			compose.ServiceLabel: service,
			compose.ProjectLabel: strings.ToLower(testProject),
		},
		State: status,
	}
}

func TestMatchLocalManifestWithoutImageData(t *testing.T) {
	// engines may omit per-manifest image data (seen with locally built,
	// never-pushed images): the lone available manifest falls back to the
	// inspect's flat platform fields instead of reporting the platform
	// unsatisfied — which made up try to pull a local-only image.
	amd64 := platforms.OnlyStrict(specs.Platform{OS: "linux", Architecture: "amd64"})
	arm64 := platforms.OnlyStrict(specs.Platform{OS: "linux", Architecture: "arm64"})
	inspect := image.InspectResponse{
		ID:           "sha256:index",
		Os:           "linux",
		Architecture: "amd64",
		Manifests: []image.ManifestSummary{
			{ID: "sha256:lone", Kind: image.ManifestKindImage, Available: true},
		},
	}

	id, satisfied := matchLocalManifest(inspect, amd64)
	assert.Equal(t, id, "sha256:lone")
	assert.Assert(t, satisfied, "flat platform fields match the requested platform")

	id, satisfied = matchLocalManifest(inspect, arm64)
	assert.Equal(t, id, "sha256:lone")
	assert.Assert(t, !satisfied, "flat platform fields don't match the requested platform")
}
