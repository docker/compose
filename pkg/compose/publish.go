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
	"github.com/distribution/reference"
	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"

	"github.com/docker/compose/v5/internal/publish"
	"github.com/docker/compose/v5/internal/registry"
	"github.com/docker/compose/v5/pkg/api"
)

// publish lives in internal/publish: composeService serves it by handing that
// package the engine it acts on.

func (s *composeService) Publish(ctx context.Context, project *types.Project, repository string, options api.PublishOptions) error {
	return Run(ctx, func(ctx context.Context) error {
		return publish.New(publishBackend{s}).Publish(ctx, project, repository, options)
	}, "publish", s.events)
}

// publishBackend is the engine of a composeService, as internal/publish sees it.
type publishBackend struct {
	s *composeService
}

func (b publishBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b publishBackend) AuthProvider() registry.AuthProvider {
	return b.s.authProvider()
}

func (b publishBackend) Events() api.EventProcessor {
	return b.s.events
}

func (b publishBackend) DryRun() bool {
	return b.s.dryRun
}

func (b publishBackend) Prompt(message string, defaultValue bool) (bool, error) {
	return b.s.prompt(message, defaultValue)
}

func (b publishBackend) ImageDigestResolver(ctx context.Context) func(named reference.Named) (digest.Digest, error) {
	return imageDigestResolver(ctx, b.s.authProvider(), b.s.apiClient())
}

func (b publishBackend) Push(ctx context.Context, project *types.Project, options api.PushOptions) error {
	return b.s.push(ctx, project, options)
}
