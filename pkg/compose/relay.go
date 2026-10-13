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
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/sync/semaphore"

	relaymgr "github.com/docker/compose/v5/internal/relay" // not "relay": a test type of this package already goes by that name
	"github.com/docker/compose/v5/pkg/api"
)

// The relay containers and relay link networks standing in for provider
// services are managed in internal/relay (and deployed by internal/provider):
// composeService hands that package the engine it works with, and the
// functions below let the lifecycle code keep calling it by the names it
// always did.

func (s *composeService) relayService() *relaymgr.Service {
	return relaymgr.New(relayBackend{s})
}

func isRelayContainer(ctr container.Summary) bool {
	return relaymgr.IsContainer(ctr)
}

func checkRelayTarget(target container.Summary, serviceName, operation string) error {
	return relaymgr.CheckTarget(target, serviceName, operation)
}

// ensureRelayLinkNetworksDown returns down ops removing every provider
// service's relay link network, one op per network, each gated on the shared
// limiter like its siblings.
func (s *composeService) ensureRelayLinkNetworksDown(ctx context.Context, project *types.Project, limiter *semaphore.Weighted) []downOp {
	relays := s.relayService()
	networks, err := relays.LinkNetworks(ctx, project.Name)
	if err != nil {
		// surfaced by the op rather than aborting down: the rest of the
		// project's cleanup is independent of this lookup
		return []downOp{func() error { return err }}
	}
	ops := make([]downOp, 0, len(networks))
	for _, n := range networks {
		ops = append(ops, func() error {
			if err := acquireSlot(ctx, limiter); err != nil {
				return err
			}
			defer releaseSlot(limiter)
			return relays.RemoveLinkNetwork(ctx, n)
		})
	}
	return ops
}

// relayBackend is the engine of a composeService, as internal/relay sees it.
type relayBackend struct {
	s *composeService
}

func (b relayBackend) APIClient() client.APIClient {
	return b.s.apiClient()
}

func (b relayBackend) Events() api.EventProcessor {
	return b.s.events
}

func (b relayBackend) IsDesktopIntegrationActive(ctx context.Context) (bool, error) {
	return b.s.isDesktopIntegrationActive(ctx)
}
