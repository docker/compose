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
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/semaphore"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestParseEndpointMessage(t *testing.T) {
	port, upstream, err := parseEndpointMessage("80=host.docker.internal:49152")
	assert.NilError(t, err)
	assert.Equal(t, port, 80)
	assert.Equal(t, upstream, "host.docker.internal:49152")

	for _, invalid := range []string{"80", "abc=host:1", "0=host:1", "80=nohostport"} {
		_, _, err := parseEndpointMessage(invalid)
		assert.Assert(t, err != nil, "expected error for %q", invalid)
	}
}

// actualNetworks (compose.go) indexes discovered networks by their
// NetworkLabel value to rebuild project.Networks when down runs without an
// explicit compose file. A relay-link network never carries that label —
// it is never one of the project's own declared networks — so folding it
// in the same way would corrupt the map with a bogus empty-key entry
// instead of a real one. It must be excluded, not merely fall through with
// an empty key.
func TestActualNetworksExcludesRelayLinkNetwork(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{
			{Network: network.Network{
				Name:   "p_default",
				Labels: map[string]string{api.NetworkLabel: "default"},
			}},
			{Network: network.Network{
				Name:   "p_db_relay",
				Labels: map[string]string{api.ProjectLabel: "p", api.ServiceLabel: "db", api.RelayNetworkLabel: "true"},
			}},
		},
	}, nil)

	networks, err := svc.actualNetworks(t.Context(), "p")
	assert.NilError(t, err)
	assert.Equal(t, len(networks), 1)
	_, ok := networks["default"]
	assert.Assert(t, ok)
	_, ok = networks[""]
	assert.Assert(t, !ok, "the relay-link network must not fold into a bogus empty-key entry")
}

// ensureRelayLinkNetworksDown removes every provider service's relay link
// network on a full `down` — the path relay container removal itself
// already takes generically (matched by ServiceLabel), but that a project's
// own ensureNetworksDown never covers, since this network is never one of
// project.Networks.
// ensureRelayLinkNetworksDown looks up every relay-link network for the
// project directly by label — never by walking project.Services and
// checking Provider, which a project reconstructed from live containers
// (docker compose down without an explicit compose file) never
// repopulates. Services carries no Provider info at all here, on purpose:
// this is exactly that reconstructed shape, and the cleanup must still
// find and remove the network.
func TestEnsureRelayLinkNetworksDown(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name: "p",
		Services: types.Services{
			"db":  {Name: "db"},
			"app": {Name: "app"},
		},
	}

	apiMock.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("p").Add("label", api.RelayNetworkLabel),
	}).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)
	apiMock.EXPECT().NetworkRemove(gomock.Any(), "net-1", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	ops := svc.ensureRelayLinkNetworksDown(t.Context(), project, nil)
	assert.Equal(t, len(ops), 1)
	for _, op := range ops {
		assert.NilError(t, op())
	}
}

// With no relay-link network there is no op at all, so down's "No resource
// found to remove" warning stays reachable for a project with nothing else.
func TestEnsureRelayLinkNetworksDownNoNetworkNoOp(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)

	assert.Equal(t, len(svc.ensureRelayLinkNetworksDown(t.Context(), &types.Project{Name: "p"}, nil)), 0)
}

// A failing lookup must not abort down's other cleanup: it comes back as an
// op carrying the error, run alongside the rest.
func TestEnsureRelayLinkNetworksDownSurfacesListError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).
		Return(client.NetworkListResult{}, errors.New("daemon unavailable"))

	ops := svc.ensureRelayLinkNetworksDown(t.Context(), &types.Project{Name: "p"}, nil)
	assert.Equal(t, len(ops), 1)
	assert.ErrorContains(t, ops[0](), "daemon unavailable")
}

// A network whose relay container removal hasn't been reflected by the
// daemon's async disconnect yet must be left in place, not fail the whole
// down — a later down retries and finds it gone.
func TestEnsureRelayLinkNetworksDownToleratesStillInUse(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)
	apiMock.EXPECT().NetworkRemove(gomock.Any(), "net-1", gomock.Any()).
		Return(client.NetworkRemoveResult{}, conflictError{})

	ops := svc.ensureRelayLinkNetworksDown(t.Context(), &types.Project{Name: "p"}, nil)
	assert.Equal(t, len(ops), 1)
	assert.NilError(t, ops[0]())
}

// One op per network: a failure removing one relay-link network must not
// stop the others from being cleaned up.
func TestEnsureRelayLinkNetworksDownOpsAreIndependent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{
			{Network: network.Network{ID: "net-1", Name: "p_db_relay"}},
			{Network: network.Network{ID: "net-2", Name: "p_cache_relay"}},
		},
	}, nil)
	apiMock.EXPECT().NetworkRemove(gomock.Any(), "net-1", gomock.Any()).
		Return(client.NetworkRemoveResult{}, errors.New("transient daemon error"))
	apiMock.EXPECT().NetworkRemove(gomock.Any(), "net-2", gomock.Any()).Return(client.NetworkRemoveResult{}, nil)

	ops := svc.ensureRelayLinkNetworksDown(t.Context(), &types.Project{Name: "p"}, nil)
	assert.Equal(t, len(ops), 2)
	assert.ErrorContains(t, ops[0](), "transient daemon error")
	assert.NilError(t, ops[1]())
}

// Like its siblings, every relay-link network removal gates itself on the
// shared down limiter: with no slot available, the op never reaches the
// engine.
func TestEnsureRelayLinkNetworksDownHonorsLimiter(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)

	limiter := semaphore.NewWeighted(1)
	assert.NilError(t, limiter.Acquire(t.Context(), 1))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ops := svc.ensureRelayLinkNetworksDown(ctx, &types.Project{Name: "p"}, limiter)
	assert.Equal(t, len(ops), 1)
	assert.ErrorIs(t, ops[0](), context.Canceled)
}
