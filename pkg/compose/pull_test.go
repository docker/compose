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
	"io"
	"iter"
	"sort"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// scheduledDependentImages runs addDependentImagePulls and returns the
// dependent image references it scheduled for pull, sorted for deterministic
// assertions.
func scheduledDependentImages(t *testing.T, project *types.Project, present map[string]api.ImageSummary) []string {
	t.Helper()
	needPull := map[string]types.ServiceConfig{}
	scheduled := map[string]bool{}
	// Seed scheduled with the service images, as pullRequiredImages does before
	// calling addDependentImagePulls.
	for _, service := range project.Services {
		scheduled[service.Image] = true
	}
	if err := addDependentImagePulls(project, present, needPull, scheduled); err != nil {
		t.Fatal(err)
	}
	var images []string
	for _, s := range needPull {
		images = append(images, s.Image)
	}
	sort.Strings(images)
	return images
}

func serviceWithHook(name, img, policy string) types.ServiceConfig {
	return types.ServiceConfig{
		Name:     name,
		PreStart: []types.PreStartHook{{ContainerSpec: types.ContainerSpec{Image: "init:latest"}}},
		ContainerSpec: types.ContainerSpec{
			Image:      img,
			PullPolicy: policy,
		},
	}
}

func serviceWithVolume(img, policy, volSource string) types.ServiceConfig {
	return types.ServiceConfig{
		Name: "web",
		ContainerSpec: types.ContainerSpec{
			Image:      img,
			PullPolicy: policy,
			Volumes: []types.ServiceVolumeConfig{
				{Type: types.VolumeTypeImage, Source: volSource, Target: "/data"},
			},
		},
	}
}

// TestAddDependentImagePulls_AlwaysForcesPresentHook covers the docker-agent
// finding: `pull_policy: always` must re-pull a hook image even when it is
// already present locally, mirroring how the service image is force-pulled.
func TestAddDependentImagePulls_AlwaysForcesPresentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyAlways)},
	}
	present := map[string]api.ImageSummary{"init:latest": {ID: "sha256:present"}}

	assert.DeepEqual(t, scheduledDependentImages(t, project, present), []string{"init:latest"})
}

// TestAddDependentImagePulls_MissingPolicySkipsPresentHook verifies a present
// hook image is not re-pulled under the default (pull-if-missing) behavior.
func TestAddDependentImagePulls_MissingPolicySkipsPresentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing)},
	}
	present := map[string]api.ImageSummary{"init:latest": {ID: "sha256:present"}}

	assert.Equal(t, len(scheduledDependentImages(t, project, present)), 0)
}

// TestAddDependentImagePulls_MissingPolicyPullsAbsentHook verifies an absent
// hook image is pulled under the default policy.
func TestAddDependentImagePulls_MissingPolicyPullsAbsentHook(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing)},
	}

	assert.DeepEqual(t, scheduledDependentImages(t, project, map[string]api.ImageSummary{}), []string{"init:latest"})
}

// TestAddDependentImagePulls_NeverSkips verifies `pull_policy: never` never
// schedules a hook pull, even for an absent image.
func TestAddDependentImagePulls_NeverSkips(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", types.PullPolicyNever)},
	}

	assert.Equal(t, len(scheduledDependentImages(t, project, map[string]api.ImageSummary{})), 0)
}

// TestAddDependentImagePulls_VolumeOnlyImage verifies that a type=image volume
// source that is not present locally gets scheduled for pull.
func TestAddDependentImagePulls_VolumeOnlyImage(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web": serviceWithVolume("nginx:alpine", types.PullPolicyMissing, "myorg/assets:latest"),
		},
	}

	assert.DeepEqual(t, scheduledDependentImages(t, project, map[string]api.ImageSummary{}), []string{"myorg/assets:latest"})
}

// TestAddDependentImagePulls_ServiceAndVolumeDedup verifies that when the same
// image is used by a service and as a volume source, the service entry wins
// (via the scheduled map) and the image is not pulled twice.
func TestAddDependentImagePulls_ServiceAndVolumeDedup(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name: "assets",
				ContainerSpec: types.ContainerSpec{
					Image:      "myorg/assets:latest",
					PullPolicy: types.PullPolicyAlways,
				},
			},
			"web": serviceWithVolume("nginx:alpine", types.PullPolicyMissing, "myorg/assets:latest"),
		},
	}
	// The service image "myorg/assets:latest" is already in scheduled (seeded
	// by scheduledDependentImages), so the volume reference is deduplicated.
	assert.Equal(t, len(scheduledDependentImages(t, project, map[string]api.ImageSummary{})), 0)
}

// TestAddDependentImagePulls_BuildableVolumeSkipped verifies that a volume
// image produced by a buildable service in the project is not scheduled for
// pull, avoiding the --ignore-buildable regression in the up path.
func TestAddDependentImagePulls_BuildableVolumeSkipped(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name: "assets",
				ContainerSpec: types.ContainerSpec{
					Image:      "demo/assets:dev",
					PullPolicy: types.PullPolicyMissing,
				},
				WorkloadSpec: types.WorkloadSpec{
					Build: &types.BuildConfig{Context: "."},
				},
			},
			"web": serviceWithVolume("nginx:alpine", types.PullPolicyMissing, "demo/assets:dev"),
		},
	}
	// Even though demo/assets:dev is not present locally, it should be skipped
	// because the "assets" service can build it.
	needPull := map[string]types.ServiceConfig{}
	scheduled := map[string]bool{}
	scheduled["nginx:alpine"] = true
	err := addDependentImagePulls(project, map[string]api.ImageSummary{}, needPull, scheduled)
	assert.NilError(t, err)
	assert.Equal(t, len(needPull), 0)
}

// TestAddDependentImagePulls_ServicePolicyNeverWins verifies that when an image
// is defined by a service with pull_policy: never, a volume referencing the
// same image is not scheduled for pull in the up path.
func TestAddDependentImagePulls_ServicePolicyNeverWins(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name: "assets",
				ContainerSpec: types.ContainerSpec{
					Image:      "myorg/assets:latest",
					PullPolicy: types.PullPolicyNever,
				},
			},
			"web": serviceWithVolume("nginx:alpine", types.PullPolicyMissing, "myorg/assets:latest"),
		},
	}
	needPull := map[string]types.ServiceConfig{}
	scheduled := map[string]bool{}
	scheduled["nginx:alpine"] = true
	err := addDependentImagePulls(project, map[string]api.ImageSummary{}, needPull, scheduled)
	assert.NilError(t, err)
	assert.Equal(t, len(needPull), 0)
}

// fakePullResponse is an empty, already-complete pull stream.
type fakePullResponse struct{}

func (fakePullResponse) Read([]byte) (int, error) { return 0, io.EOF }
func (fakePullResponse) Close() error             { return nil }
func (fakePullResponse) Wait(context.Context) error {
	return nil
}

func (fakePullResponse) JSONMessages(context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(func(jsonstream.Message, error) bool) {}
}

// TestPullServiceImageUsesContentDigest verifies the pull path resolves the
// pulled image's identity with the same contentDigest scheme
// getLocalImagesDigests uses for already-local images. Both values feed the
// com.docker.compose.image label that detects stale containers, so when the
// pull path returned the raw inspect ID instead (the index digest, under the
// containerd store with a tag@digest ref), the first up after the pulling up
// saw a phantom image change and recreated every container once.
func TestPullRequiredImagesUsesContentDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI, tested := newTestComposeService(t, mockCtrl, "1.48")

	ref := "foo:1@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// two services pull the same image (e.g. for different platforms): the
	// shared summary digest is resolved once, after all pulls completed, so
	// it cannot depend on pull completion order
	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), ref, gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(2)
	inspect := image.InspectResponse{
		ID: "sha256:index",
		Manifests: []image.ManifestSummary{
			imageManifest("sha256:image", "amd64", true),
			attestationManifest(),
		},
	}
	mockAPI.EXPECT().
		ImageInspect(anyCancellableContext(), ref, gomock.Any()).
		Return(client.ImageInspectResult{InspectResponse: inspect}, nil).
		Times(1)

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web":    {Name: "web", ContainerSpec: types.ContainerSpec{Image: ref}},
			"pinned": {Name: "pinned", ContainerSpec: types.ContainerSpec{Image: ref, Platform: "linux/amd64"}},
		},
	}
	images := map[string]api.ImageSummary{}
	err := tested.pullRequiredImages(t.Context(), project, images, true)
	assert.NilError(t, err)
	assert.Equal(t, images[ref].ID, "sha256:image")
}

// TestAddDependentImagePulls_DedupsSharedHookImage verifies a hook image shared by
// several services is scheduled at most once.
func TestAddDependentImagePulls_DedupsSharedHookImage(t *testing.T) {
	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web": serviceWithHook("web", "web:latest", types.PullPolicyMissing),
			"api": serviceWithHook("api", "api:latest", types.PullPolicyMissing),
		},
	}

	assert.DeepEqual(t, scheduledDependentImages(t, project, map[string]api.ImageSummary{}), []string{"init:latest"})
}

func TestShouldPullImage(t *testing.T) {
	present := map[string]api.ImageSummary{
		"web:1":      {LastTagTime: time.Now()},
		"web:latest": {LastTagTime: time.Now()},
		"old:1":      {LastTagTime: time.Now().Add(-48 * time.Hour)},
	}
	svc := func(image, policy string) types.ServiceConfig {
		return types.ServiceConfig{Name: "web", ContainerSpec: types.ContainerSpec{Image: image, PullPolicy: policy}}
	}

	t.Run("no explicit policy always refreshes", func(t *testing.T) {
		pull, _, err := shouldPullImage(svc("web:1", ""), present)
		assert.NilError(t, err)
		assert.Assert(t, pull)
	})

	t.Run("never and build skip", func(t *testing.T) {
		for _, policy := range []string{types.PullPolicyNever, types.PullPolicyBuild} {
			pull, _, err := shouldPullImage(svc("web:1", policy), present)
			assert.NilError(t, err)
			assert.Assert(t, !pull)
		}
	})

	t.Run("missing skips a present image", func(t *testing.T) {
		pull, _, err := shouldPullImage(svc("web:1", types.PullPolicyMissing), present)
		assert.NilError(t, err)
		assert.Assert(t, !pull)

		pull, _, err = shouldPullImage(svc("absent:1", types.PullPolicyMissing), present)
		assert.NilError(t, err)
		assert.Assert(t, pull)
	})

	t.Run("missing still refreshes a present latest tag", func(t *testing.T) {
		// deliberate exception: `latest` is expected to move, so the pull is
		// triggered anyway and the daemon's registry negotiation decides
		// (a no-op when the local image is already up to date)
		for _, image := range []string{"web:latest", "web"} {
			pull, _, err := shouldPullImage(svc(image, types.PullPolicyMissing), map[string]api.ImageSummary{
				image: {LastTagTime: time.Now()},
			})
			assert.NilError(t, err)
			assert.Assert(t, pull, "present %s must still be refreshed", image)
		}
	})

	t.Run("refresh policies are due on explicit pull", func(t *testing.T) {
		// an explicit `compose pull` is the only way to force a refresh ahead
		// of the daily/weekly/every_N window, so the window is treated as due
		pull, _, err := shouldPullImage(svc("web:1", "daily"), present)
		assert.NilError(t, err)
		assert.Assert(t, pull, "explicit pull refreshes ahead of the window")

		pull, _, err = shouldPullImage(svc("old:1", "daily"), present)
		assert.NilError(t, err)
		assert.Assert(t, pull, "image older than the window must be refreshed")

		pull, _, err = shouldPullImage(svc("absent:1", "weekly"), present)
		assert.NilError(t, err)
		assert.Assert(t, pull, "absent image must be pulled")
	})

	t.Run("invalid refresh spec errors", func(t *testing.T) {
		_, _, err := shouldPullImage(svc("web:1", "every_bogus"), present)
		assert.Assert(t, err != nil)
	})
}

// TestAddDependentImagePulls_RefreshWindow: hook images inherit the parent
// service's daily/weekly/every_N policy through the same mustPull interpreter
// as the service image, so `up` honors the refresh window for them too.
func TestAddDependentImagePulls_RefreshWindow(t *testing.T) {
	project := &types.Project{
		Name:     "demo",
		Services: types.Services{"web": serviceWithHook("web", "web:latest", "daily")},
	}

	fresh := map[string]api.ImageSummary{
		"init:latest": {ID: "sha256:present", LastTagTime: time.Now()},
	}
	assert.DeepEqual(t, scheduledDependentImages(t, project, fresh), []string(nil))

	stale := map[string]api.ImageSummary{
		"init:latest": {ID: "sha256:present", LastTagTime: time.Now().Add(-48 * time.Hour)},
	}
	assert.DeepEqual(t, scheduledDependentImages(t, project, stale), []string{"init:latest"})
}

// TestShouldPullImageProvider: a service declaring both provider: and image:
// still gets its image refreshed by an explicit pull, while a provider-only
// service is skipped.
func TestShouldPullImageProvider(t *testing.T) {
	images := map[string]api.ImageSummary{}

	pull, _, err := shouldPullImage(types.ServiceConfig{
		Name: "db",

		Provider: &types.ServiceProviderConfig{Type: "acme"}, ContainerSpec: types.ContainerSpec{Image: "db:1"},
	}, images)
	assert.NilError(t, err)
	assert.Assert(t, pull, "provider service with a declared image must be pulled")

	pull, _, err = shouldPullImage(types.ServiceConfig{
		Name:     "db",
		Provider: &types.ServiceProviderConfig{Type: "acme"},
	}, images)
	assert.NilError(t, err)
	assert.Assert(t, !pull, "provider service without image has nothing to pull")
}

// TestImageDigestResolverUsesDistributionDigest locks the distinction between
// the two digest kinds compose manipulates: pinning a reference for a
// reproducible compose model (publish / config --resolve-image-digests) must
// use the registry descriptor digest — the multi-platform index — and never
// the local platform-specific content digest used for staleness comparison.
// The mock declares no ImageInspect expectation, so any attempt to resolve
// through the local content-digest producer fails the test.
func TestImageDigestResolverUsesDistributionDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI := mocks.NewMockAPIClient(mockCtrl)

	const indexDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mockAPI.EXPECT().
		DistributionInspect(gomock.Any(), "docker.io/library/foo:1", gomock.Any()).
		Return(client.DistributionInspectResult{
			DistributionInspect: registrytypes.DistributionInspect{
				Descriptor: ocispec.Descriptor{Digest: digest.Digest(indexDigest)},
			},
		}, nil)

	named, err := reference.ParseDockerRef("foo:1")
	assert.NilError(t, err)
	// The unexported variant keeps the test away from a real Docker Desktop.
	resolved, err := imageDigestResolver(t.Context(), &configfile.ConfigFile{}, mockAPI)(named)
	assert.NilError(t, err)
	assert.Equal(t, resolved.String(), indexDigest)
}

// TestPull_IgnoreBuildable_SkipsBuildableVolumeImage verifies that running
// `compose pull --ignore-buildable` skips dependent volume images produced by
// buildable services in the project, avoiding registry pull attempts and failures.
func TestPull_IgnoreBuildable_SkipsBuildableVolumeImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI, tested := newTestComposeService(t, mockCtrl, "1.48")

	mockAPI.EXPECT().
		ImageInspect(anyCancellableContext(), gomock.Any(), gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound).
		AnyTimes()

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name:          "assets",
				ContainerSpec: types.ContainerSpec{Image: "demo/assets:dev"},
				WorkloadSpec:  types.WorkloadSpec{Build: &types.BuildConfig{Context: "."}},
			},
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Image: "nginx:alpine",
					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeImage, Source: "demo/assets:dev", Target: "/srv/static"},
					},
				},
			},
		},
	}

	// nginx:alpine must be pulled, but demo/assets:dev must NOT be pulled
	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "nginx:alpine", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)

	err := tested.Pull(t.Context(), project, api.PullOptions{IgnoreBuildable: true})
	assert.NilError(t, err)
}

// TestPull_VolumeOnlyImage verifies that running compose pull pulls an image
// used only as a type: image volume.
func TestPull_VolumeOnlyImage(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI, tested := newTestComposeService(t, mockCtrl, "1.48")

	mockAPI.EXPECT().
		ImageInspect(anyCancellableContext(), gomock.Any(), gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound).
		AnyTimes()

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Image: "nginx:alpine",
					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeImage, Source: "myorg/assets:latest", Target: "/srv/static"},
					},
				},
			},
		},
	}

	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "nginx:alpine", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)
	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "myorg/assets:latest", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)

	err := tested.Pull(t.Context(), project, api.PullOptions{})
	assert.NilError(t, err)
}

// TestPull_SameImageUsedByServiceAndVolume verifies that when the same image is
// used by a service and as a volume source, it is pulled only once (deduplicated).
func TestPull_SameImageUsedByServiceAndVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI, tested := newTestComposeService(t, mockCtrl, "1.48")

	mockAPI.EXPECT().
		ImageInspect(anyCancellableContext(), gomock.Any(), gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound).
		AnyTimes()

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name: "assets",
				ContainerSpec: types.ContainerSpec{
					Image: "myorg/assets:latest",
				},
			},
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Image: "nginx:alpine",
					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeImage, Source: "myorg/assets:latest", Target: "/srv/static"},
					},
				},
			},
		},
	}

	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "myorg/assets:latest", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)
	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "nginx:alpine", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)

	err := tested.Pull(t.Context(), project, api.PullOptions{})
	assert.NilError(t, err)
}

// TestPull_SameImageUsedByServiceAndVolume_ServicePolicyWins verifies that when
// the service defines pull_policy: never, the service policy wins and the volume
// image is not pulled.
func TestPull_SameImageUsedByServiceAndVolume_ServicePolicyWins(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI, tested := newTestComposeService(t, mockCtrl, "1.48")

	mockAPI.EXPECT().
		ImageInspect(anyCancellableContext(), gomock.Any(), gomock.Any()).
		Return(client.ImageInspectResult{}, errdefs.ErrNotFound).
		AnyTimes()

	project := &types.Project{
		Name: "demo",
		Services: types.Services{
			"assets": {
				Name: "assets",
				ContainerSpec: types.ContainerSpec{
					Image:      "myorg/assets:latest",
					PullPolicy: types.PullPolicyNever,
				},
			},
			"web": {
				Name: "web",
				ContainerSpec: types.ContainerSpec{
					Image: "nginx:alpine",
					Volumes: []types.ServiceVolumeConfig{
						{Type: types.VolumeTypeImage, Source: "myorg/assets:latest", Target: "/srv/static"},
					},
				},
			},
		},
	}

	// Only nginx:alpine should be pulled; myorg/assets:latest must NOT be pulled
	mockAPI.EXPECT().
		ImagePull(anyCancellableContext(), "nginx:alpine", gomock.Any()).
		Return(fakePullResponse{}, nil).
		Times(1)

	err := tested.Pull(t.Context(), project, api.PullOptions{})
	assert.NilError(t, err)
}
