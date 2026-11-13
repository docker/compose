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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	clitypes "github.com/docker/cli/cli/config/types"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

// stubAuth is a comparable registry.AuthProvider (a func-typed one is not).
type stubAuth struct{}

func (*stubAuth) GetAuthConfig(string) (clitypes.AuthConfig, error) {
	return clitypes.AuthConfig{Username: "user", Password: "secret"}, nil
}

// TestPublishBackendWiring pins the adapter between a composeService and
// internal/publish: the publish tests run against their own fake backend, so
// an adapter that stopped following the service (a refused prompt answered
// "yes", a dry run not honored, the public Push opening a second "push"
// bracket inside the "publish" one, a digest resolved with another context)
// would otherwise go unnoticed.
func TestPublishBackendWiring(t *testing.T) {
	api, s := newTestComposeService(t, gomock.NewController(t), "1.48")
	auth := &stubAuth{}
	events := &bracketRecorder{}
	s.auth = auth
	s.events = events
	b := publishBackend{s}

	t.Run("service accessors", func(t *testing.T) {
		assert.Assert(t, b.APIClient() == client.APIClient(api))
		assert.Assert(t, b.AuthProvider() == auth)
		assert.Assert(t, b.Events() == compose.EventProcessor(events))
	})

	t.Run("dry run follows the service", func(t *testing.T) {
		s.dryRun = true
		assert.Assert(t, b.DryRun())
		s.dryRun = false
		assert.Assert(t, !b.DryRun())
	})

	t.Run("a refused prompt stays refused", func(t *testing.T) {
		var asked string
		var asDefault bool
		s.prompt = func(message string, defaultValue bool) (bool, error) {
			asked, asDefault = message, defaultValue
			return false, nil
		}
		answer, err := b.Prompt("publish?", true)
		assert.NilError(t, err)
		assert.Assert(t, !answer)
		assert.Equal(t, asked, "publish?")
		assert.Assert(t, asDefault)
	})

	t.Run("push reaches the unexported push with its options", func(t *testing.T) {
		project := &types.Project{Services: types.Services{"web": {Name: "web"}}}

		err := b.Push(t.Context(), project, compose.PushOptions{ImageMandatory: true})

		assert.ErrorContains(t, err, "mandatory")
		// publish already runs inside the "publish" bracket: the public Push
		// would open a second, "push" one.
		assert.Equal(t, len(events.entries), 0, "unexpected events: %v", events.entries)
	})

	t.Run("image digests resolve with the service's auth and the caller's context", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(t.Context(), ctxKey{}, "caller")
		const indexDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		api.EXPECT().
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

		resolved, err := b.ImageDigestResolver(ctx)(named)

		assert.NilError(t, err)
		assert.Equal(t, resolved.String(), indexDigest)
	})
}
