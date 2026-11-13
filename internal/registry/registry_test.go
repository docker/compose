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

package registry

import (
	"context"
	"testing"

	"github.com/distribution/reference"
	clitypes "github.com/docker/cli/cli/config/types"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

// TestImageDigestResolverUsesDistributionDigest locks the distinction between
// the two digest kinds compose manipulates: pinning a reference for a
// reproducible compose model (publish / config --resolve-image-digests) must
// use the registry descriptor digest — the multi-platform index — and never
// the local platform-specific content digest used for staleness comparison.
// The mock declares no ImageInspect expectation, so any attempt to resolve
// through the local content-digest producer fails the test. The lookup must
// also run with the caller's context and the credentials of the given auth.
func TestImageDigestResolverUsesDistributionDigest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI := mocks.NewMockAPIClient(mockCtrl)

	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "caller")
	const indexDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mockAPI.EXPECT().
		DistributionInspect(ctx, "docker.io/library/foo:1", gomock.Cond(func(options client.DistributionInspectOptions) bool {
			return options.EncodedRegistryAuth != ""
		})).
		Return(client.DistributionInspectResult{
			DistributionInspect: registrytypes.DistributionInspect{
				Descriptor: ocispec.Descriptor{Digest: digest.Digest(indexDigest)},
			},
		}, nil)

	named, err := reference.ParseDockerRef("foo:1")
	assert.NilError(t, err)
	auth := AuthProviderFunc(func(string) (clitypes.AuthConfig, error) {
		return clitypes.AuthConfig{Username: "user", Password: "secret"}, nil
	})
	resolved, err := ImageDigestResolver(ctx, auth, mockAPI)(named)
	assert.NilError(t, err)
	assert.Equal(t, resolved.String(), indexDigest)
}
