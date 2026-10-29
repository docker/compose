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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal/progress"
	"github.com/docker/compose/v5/pkg/mocks"
)

// providerSourceService builds a ServiceConfig by assignment: Image and
// PullPolicy are fields promoted from the embedded ContainerSpec, which
// struct literals cannot set before go1.27.
func providerSourceService(build *types.BuildConfig, imageRef, pullPolicy string) types.ServiceConfig {
	var s types.ServiceConfig
	s.Build = build
	s.Image = imageRef
	s.PullPolicy = pullPolicy
	return s
}

func TestProviderImageSource(t *testing.T) {
	build := &types.BuildConfig{Context: "."}
	tests := []struct {
		name      string
		service   types.ServiceConfig
		justBuilt bool
		want      string
	}{
		{"no build: registry authority", providerSourceService(nil, "nginx", ""), false, providerImageSourceRegistry},
		{"build without image: local-only name", providerSourceService(build, "", ""), false, providerImageSourceLocal},
		{"build just run: local wins for this invocation", providerSourceService(build, "repo/app", ""), true, providerImageSourceLocal},
		{"build as CI recipe, default policy: registry authority", providerSourceService(build, "repo/app", ""), false, providerImageSourceRegistry},
		{"pull_policy build: local authority", providerSourceService(build, "repo/app", types.PullPolicyBuild), false, providerImageSourceLocal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, providerImageSource(tc.service, tc.justBuilt), tc.want)
		})
	}
}

// TestEnsureProviderImages_SkipsPureResourceProvider: a provider service
// declaring neither image: nor build: (legal — e.g. a remote-resource
// provider like an RDS instance) has nothing to distribute. It must never
// reach the pull command at all — reaching it would hand the provider a
// fabricated "<project>-<service>" reference (GetImageNameOrDefault) under
// source=registry (providerImageSource with Build == nil), a name that was
// never real.
func TestEnsureProviderImages_SkipsPureResourceProvider(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	cli := mocks.NewMockCli(mockCtrl)
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	svc, err := NewComposeService(cli, WithEventProcessor(progress.Noop{}))
	assert.NilError(t, err)

	project := &types.Project{
		Name: "proj",
		Services: types.Services{
			"db": {
				Name: "db",
				// a provider type that resolves to no plugin binary at all:
				// if the skip guard didn't fire, runProviderPull would reach
				// getPluginBinaryPath and fail, surfacing as a non-nil error
				// here instead of this test's expected nil.
				Provider: &types.ServiceProviderConfig{Type: "no-such-provider-plugin"},
			},
		},
	}
	err = svc.(*composeService).ensureProviderImages(t.Context(), project, nil, providerPullPolicyMissing)
	assert.NilError(t, err, "a provider with neither image nor build has nothing to distribute and must not invoke pull")
}

// TestLocalImageFacts_UsesManifestAwareDigest: --digest must be the
// manifest-scoped content digest (localContentDigest), not the raw inspect
// ID. Under BuildKit provenance attestations the raw ID covers the
// attestation manifest too and changes on every build even when the runnable
// image content didn't (#13636) — which would make the provider re-sync on
// every up for no reason, defeating docs/extension.md's "identity test"
// contract for --digest.
func TestLocalImageFacts_UsesManifestAwareDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api, tested := newTestComposeService(t, mockCtrl, "1.48")

	api.EXPECT().
		ImageInspect(anyCancellableContext(), "proj-db", gomock.Any()).
		Return(client.ImageInspectResult{InspectResponse: image.InspectResponse{
			ID:       "sha256:index",
			Metadata: image.Metadata{},
			Manifests: []image.ManifestSummary{
				imageManifest("sha256:image", "amd64", true),
				attestationManifest(),
			},
		}}, nil)

	digest, _, ok := tested.localImageFacts(t.Context(), "proj-db", "")
	assert.Assert(t, ok)
	assert.Equal(t, digest, "sha256:image", "--digest must be the image-kind manifest digest, not the attestation-covering index ID")
}
