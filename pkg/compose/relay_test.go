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
	"net/netip"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/semaphore"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// COMPOSE_RELAY_IMAGE lets internal-registry users substitute their own copy
// of the relay image for the Docker Hub default — and the identity hash
// covers the image, so switching it recreates existing relays.
func TestRelayImageOverride(t *testing.T) {
	t.Setenv("COMPOSE_RELAY_IMAGE", "")
	assert.Equal(t, relayImage(), defaultRelayImage)
	defaultIdentity := relayIdentity("80=host.docker.internal:1")

	t.Setenv("COMPOSE_RELAY_IMAGE", "registry.corp.example/infra/compose-relay:v1")
	assert.Equal(t, relayImage(), "registry.corp.example/infra/compose-relay:v1")
	assert.Assert(t, relayIdentity("80=host.docker.internal:1") != defaultIdentity)
}

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

// relayRoutesSpec is canonically ordered: it is both the relay's runtime
// configuration and the identity hashed into its label, so map iteration
// order must never leak into it.
func TestRelayRoutesSpec(t *testing.T) {
	routes := relayRoutesSpec(map[int]string{
		9000: "host.docker.internal:30001",
		80:   "host.docker.internal:49152",
	})
	assert.Equal(t, routes, "80=host.docker.internal:49152,9000=host.docker.internal:30001")

	id1 := relayIdentity(routes)
	id2 := relayIdentity(routes)
	assert.Equal(t, id1, id2)
	assert.Assert(t, id1 != relayIdentity("80=host.docker.internal:49153"))
}

// Providers express endpoints from the host's perspective, the relay dials
// from a container: host-relative addresses must be rewritten to
// host.docker.internal, everything else passes verbatim.
func TestRelayUpstream(t *testing.T) {
	for endpoint, want := range map[string]string{
		"localhost:5734":            "host.docker.internal:5734",
		"LOCALHOST:5734":            "host.docker.internal:5734",
		"127.0.0.1:5734":            "host.docker.internal:5734",
		"127.1.2.3:5734":            "host.docker.internal:5734",
		"[::1]:5734":                "host.docker.internal:5734",
		"0.0.0.0:5734":              "host.docker.internal:5734",
		"[::]:5734":                 "host.docker.internal:5734",
		":5734":                     "host.docker.internal:5734",
		"192.168.1.10:5734":         "192.168.1.10:5734",
		"[fdcb::2]:5734":            "[fdcb::2]:5734",
		"some.host.corp:5734":       "some.host.corp:5734",
		"host.docker.internal:5734": "host.docker.internal:5734",
	} {
		assert.Equal(t, relayUpstream(endpoint), want, "endpoint %q", endpoint)
	}
}

// The rewrite happens inside relayRoutesSpec, so the relay identity hashes
// what the relay actually dials.
func TestRelayRoutesSpecRewritesHostRelativeUpstreams(t *testing.T) {
	routes := relayRoutesSpec(map[int]string{80: "localhost:5734"})
	assert.Equal(t, routes, "80=host.docker.internal:5734")
}

// The relay joins the networks of the services depending on the provider —
// its consumers — and falls back to the project default network.
func TestRelayNetworks(t *testing.T) {
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	project := &types.Project{
		Name: "p",
		Services: types.Services{
			"db": db,
			"app": {
				Name:          "app",
				WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"db": {}}},
				ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{"backend": nil}},
			},
			"web": {
				Name:          "web",
				WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"db": {}}},
				ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{"frontend": nil, "backend": nil}},
			},
			"other": {
				Name:          "other",
				ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{"private": nil}},
			},
		},
		Networks: types.Networks{"default": {}, "backend": {}, "frontend": {}, "private": {}},
	}

	assert.DeepEqual(t, relayNetworks(project, db), []string{"backend", "frontend"})

	// no consumer: fall back to the project default network
	lonely := types.ServiceConfig{Name: "lonely", Provider: &types.ServiceProviderConfig{Type: "test"}}
	assert.DeepEqual(t, relayNetworks(project, lonely), []string{"default"})
}

// relayLinkNetworkName is deterministic and namespaced by project+service,
// so two provider services in the same project — or the same service name
// in two different projects — never collide.
func TestRelayLinkNetworkName(t *testing.T) {
	assert.Equal(t, relayLinkNetworkName("p", "db"), "p_db_relay")
	assert.Assert(t, relayLinkNetworkName("p", "db") != relayLinkNetworkName("p", "cache"))
	assert.Assert(t, relayLinkNetworkName("p", "db") != relayLinkNetworkName("q", "db"))
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

// A second convergence for the same service must reuse the network it
// already created — not attempt to create it again — and report the
// gateway resolved from the EXISTING network's own inspect, not a fresh one.
func TestEnsureRelayLinkNetworkReusesExisting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{Name: "p"}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)
	inspect := client.NetworkInspectResult{}
	inspect.Network.IPAM.Config = []network.IPAMConfig{{Gateway: netip.MustParseAddr("172.20.0.1")}}
	apiMock.EXPECT().NetworkInspect(gomock.Any(), "p_db_relay", gomock.Any()).Return(inspect, nil)

	name, gateway, err := svc.ensureRelayLinkNetwork(t.Context(), project, service)
	assert.NilError(t, err)
	assert.Equal(t, name, "p_db_relay")
	assert.Equal(t, gateway, "172.20.0.1")
}

// Absent, the network is created bridge/internal, labeled for later lookup
// (find-or-create, and removeRelayLinkNetwork's own filter), and its
// gateway resolved from the fresh inspect.
func TestEnsureRelayLinkNetworkCreatesWhenAbsent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{Name: "p"}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)
	var created client.NetworkCreateOptions
	apiMock.EXPECT().NetworkCreate(gomock.Any(), "p_db_relay", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.NetworkCreateOptions) (client.NetworkCreateResult, error) {
			created = opts
			return client.NetworkCreateResult{}, nil
		})
	inspect := client.NetworkInspectResult{}
	inspect.Network.IPAM.Config = []network.IPAMConfig{{Gateway: netip.MustParseAddr("172.21.0.1")}}
	apiMock.EXPECT().NetworkInspect(gomock.Any(), "p_db_relay", gomock.Any()).Return(inspect, nil)

	name, gateway, err := svc.ensureRelayLinkNetwork(t.Context(), project, service)
	assert.NilError(t, err)
	assert.Equal(t, name, "p_db_relay")
	assert.Equal(t, gateway, "172.21.0.1")
	assert.Equal(t, created.Driver, "bridge")
	assert.Assert(t, created.Internal)
	assert.Equal(t, created.Labels[api.ProjectLabel], "p")
	assert.Equal(t, created.Labels[api.ServiceLabel], "db")
	assert.Equal(t, created.Labels[api.RelayNetworkLabel], "true")
}

// A concurrent up for the same service creating the network first is the
// desired state, not a failure — same tolerance createNetwork already has
// for a project's own declared networks.
func TestEnsureRelayLinkNetworkToleratesConcurrentCreate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{Name: "p"}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}

	gomock.InOrder(
		apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil),
		apiMock.EXPECT().NetworkCreate(gomock.Any(), "p_db_relay", gomock.Any()).
			Return(client.NetworkCreateResult{}, conflictError{}),
		// the post-conflict label re-check: the concurrent up's network is
		// now visible, confirming the conflict was ours to adopt
		apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
			Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
		}, nil),
		apiMock.EXPECT().NetworkInspect(gomock.Any(), "p_db_relay", gomock.Any()).
			Return(client.NetworkInspectResult{}, nil),
	)

	_, _, err = svc.ensureRelayLinkNetwork(t.Context(), project, service)
	assert.NilError(t, err)
}

// A NetworkCreate conflict on the deterministic name must not be trusted
// blindly: if it comes from an unrelated, unlabeled network happening to
// share the name (e.g. a user declaring networks: {db_relay: {}}) rather
// than a concurrent up for the same service, ensureRelayLinkNetwork must
// fail instead of adopting that network into the relay's isolation
// boundary — the re-check by label (findRelayLinkNetwork) comes back empty.
func TestEnsureRelayLinkNetworkConflictWithUnrelatedNetworkFails(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{Name: "p"}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}

	gomock.InOrder(
		apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil),
		apiMock.EXPECT().NetworkCreate(gomock.Any(), "p_db_relay", gomock.Any()).
			Return(client.NetworkCreateResult{}, conflictError{}),
		apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil),
	)
	// No NetworkInspect: the label re-check coming back empty must short
	// circuit before ever inspecting the colliding network by name.

	_, _, err = svc.ensureRelayLinkNetwork(t.Context(), project, service)
	assert.ErrorContains(t, err, "already exists and is not a relay link network")
}

// removeRelayLinkNetwork is a no-op when nothing was ever created for the
// service — no NetworkRemove call, which mockCtrl.Finish would catch as an
// unexpected call anyway.
func TestRemoveRelayLinkNetworkNoopWhenAbsent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)

	assert.NilError(t, svc.removeRelayLinkNetwork(t.Context(), "p", "db"))
}

// Every caller removes the relay container first, but the daemon disconnects
// its network endpoints asynchronously: NetworkRemove can still race that
// disconnect and return a conflict. removeRelayLinkNetwork must tolerate it
// like removeServiceRelay's other best-effort cleanup steps, not fail the
// caller outright.
func TestRemoveRelayLinkNetworkToleratesActiveEndpointsConflict(t *testing.T) {
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

	assert.NilError(t, svc.removeRelayLinkNetwork(t.Context(), "p", "db"))
}

// The network disappearing between the list and the remove (a concurrent
// down) is the desired end state, not a failure.
func TestRemoveRelayLinkNetworkToleratesAlreadyGone(t *testing.T) {
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
		Return(client.NetworkRemoveResult{}, errdefs.ErrNotFound)

	assert.NilError(t, svc.removeRelayLinkNetwork(t.Context(), "p", "db"))
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

// ensureServiceRelay runs concurrently per provider service under the shared
// project mutex released before Docker API work: another goroutine may
// connect the relay to the same network in the window after our
// ContainerList snapshot. The daemon's "endpoint already exists" for that
// race is the desired state, not a failure.
func TestEnsureRelayNetworksTreatsAlreadyConnectedAsSuccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name:     "p",
		Networks: types.Networks{"frontend": {Name: "p_frontend"}},
	}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	existing := &container.Summary{ID: "relay-1"}

	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_frontend", gomock.Any()).
		Return(client.NetworkConnectResult{}, conflictError{})

	err = svc.ensureRelayNetworks(t.Context(), project, existing, service, []string{"frontend"}, "")
	assert.NilError(t, err)
}

// relayIdentity only hashes image+routes: a dependent service added later on
// a new network doesn't change it, so ensureRelayNetworks is what must catch
// up the relay's network membership — connecting only the network it isn't
// already on, leaving the one it has untouched.
func TestEnsureRelayNetworksConnectsOnlyMissingNetworks(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name: "p",
		Networks: types.Networks{
			"backend":  {Name: "p_backend"},
			"frontend": {Name: "p_frontend"},
		},
	}
	service := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	existing := &container.Summary{
		ID: "relay-1",
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*network.EndpointSettings{
				"p_backend": {},
			},
		},
	}

	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_frontend", client.NetworkConnectOptions{
		Container:      "relay-1",
		EndpointConfig: &network.EndpointSettings{Aliases: []string{"db"}},
	}).Return(client.NetworkConnectResult{}, nil)

	err = svc.ensureRelayNetworks(t.Context(), project, existing, service, []string{"backend", "frontend"}, "")
	assert.NilError(t, err)
}

// The #14224-class bug this guards: a running relay whose identity still
// matches (image+routes unchanged) used to return early without ever
// looking at network membership. A service added later on a new network
// must still get the relay connected to it, without recreating the
// container (no ContainerCreate/ContainerStart expected here).
func TestEnsureServiceRelayConnectsMissingNetworkWithoutRecreating(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name: "p",
		Networks: types.Networks{
			"backend":  {Name: "p_backend"},
			"frontend": {Name: "p_frontend"},
		},
	}
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	endpoints := map[int]string{80: "host.docker.internal:49152"}
	identity := relayIdentity(relayRoutesSpec(endpoints))

	existing := container.Summary{
		ID:     "relay-1",
		State:  container.StateRunning,
		Labels: map[string]string{api.RelayLabel: identity},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*network.EndpointSettings{
				"p_backend": {},
			},
		},
	}

	// the provider already asked get-relay-info earlier in this up (a local
	// binding), so its dedicated network already exists: ensureServiceRelay
	// only looks it up, never creates one itself (see findRelayLinkNetwork)
	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{existing}}, nil)
	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_frontend", client.NetworkConnectOptions{
		Container:      "relay-1",
		EndpointConfig: &network.EndpointSettings{Aliases: []string{"db"}},
	}).Return(client.NetworkConnectResult{}, nil)
	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_db_relay", client.NetworkConnectOptions{
		Container: "relay-1",
	}).Return(client.NetworkConnectResult{}, nil)

	err = svc.ensureServiceRelay(t.Context(), project, db, endpoints, []string{"backend", "frontend"})
	assert.NilError(t, err)
}

// A provider backing the service with a remote resource (an RDS instance,
// say) publishes an endpoint without ever having sent get-relay-info: no
// dedicated network exists for it, and ensureServiceRelay must not conjure
// one just because an endpoint was published — connecting the relay to the
// dependents' networks is the only thing it does here. No NetworkCreate,
// and the only NetworkConnect is to p_frontend — mockCtrl.Finish would
// catch either an unwanted create or an unwanted link-network connect.
func TestEnsureServiceRelaySkipsLinkNetworkForRemoteProvider(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name:     "p",
		Networks: types.Networks{"frontend": {Name: "p_frontend"}},
	}
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	endpoints := map[int]string{5432: "rds-instance.us-east-1.rds.amazonaws.com:5432"}
	identity := relayIdentity(relayRoutesSpec(endpoints))

	existing := container.Summary{
		ID:     "relay-1",
		State:  container.StateRunning,
		Labels: map[string]string{api.RelayLabel: identity},
	}

	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)
	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{existing}}, nil)
	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_frontend", client.NetworkConnectOptions{
		Container:      "relay-1",
		EndpointConfig: &network.EndpointSettings{Aliases: []string{"db"}},
	}).Return(client.NetworkConnectResult{}, nil)

	err = svc.ensureServiceRelay(t.Context(), project, db, endpoints, []string{"frontend"})
	assert.NilError(t, err)
}

// Dependents dropping to zero while a relay from a previous up is still
// running must tear that relay down, not merely skip creating a new one:
// leaving it running would strand it with stale network attachments and
// routes forever (regression: the zero-networkKeys early return used to run
// before the existing-relay lookup, so it never reached the stale container).
func TestEnsureServiceRelayRemovesExistingWhenNoDependents(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{Name: "p"}
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}
	endpoints := map[int]string{80: "host.docker.internal:49152"}

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{{ID: "relay-1", Names: []string{"/p-db-1"}}},
	}, nil)
	apiMock.EXPECT().ContainerRemove(gomock.Any(), "relay-1", client.ContainerRemoveOptions{Force: true}).
		Return(client.ContainerRemoveResult{}, nil)
	apiMock.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("p").Add("label", serviceFilter("db")).Add("label", api.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	err = svc.ensureServiceRelay(t.Context(), project, db, endpoints, nil)
	assert.NilError(t, err)
}

// Process-level commands refuse relay containers: there is no service
// process in them to act on.
func TestCheckRelayTarget(t *testing.T) {
	relay := container.Summary{Labels: map[string]string{api.RelayLabel: "abc123"}}
	err := checkRelayTarget(relay, "db", "exec")
	assert.ErrorContains(t, err, "network relay")
	assert.ErrorContains(t, err, "exec")

	regular := container.Summary{Labels: map[string]string{api.ServiceLabel: "db"}}
	assert.NilError(t, checkRelayTarget(regular, "db", "exec"))
}

// A provider publishing no endpoint on this up -- whether it never did, or a
// relay from an earlier up is now stale -- must not leave a relay routing to
// an upstream the provider no longer serves.
func TestRemoveServiceRelayRemovesExisting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{{ID: "relay-1", Names: []string{"/p-db-1"}}},
	}, nil)
	apiMock.EXPECT().ContainerRemove(gomock.Any(), "relay-1", client.ContainerRemoveOptions{Force: true}).
		Return(client.ContainerRemoveResult{}, nil)
	// removeServiceRelay always also tries the relay-link network, even
	// when nothing is left connected to it.
	apiMock.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{
		Filters: projectFilter("p").Add("label", serviceFilter("db")).Add("label", api.RelayNetworkLabel),
	}).Return(client.NetworkListResult{}, nil)

	assert.NilError(t, svc.removeServiceRelay(t.Context(), "p", "db"))
}

// The relay only dials out and forwards bytes: it gets none of Docker's
// default capabilities except NET_BIND_SERVICE, kept because a route
// commonly targets a privileged port the relay must still listen on.
func TestCreateRelayContainerDropsCapabilities(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	project := &types.Project{
		Name:     "p",
		Networks: types.Networks{"default": {Name: "p_default"}},
	}
	db := types.ServiceConfig{Name: "db", Provider: &types.ServiceProviderConfig{Type: "test"}}

	// the provider already asked get-relay-info earlier in this up, so its
	// dedicated network already exists: createRelayContainer connects the
	// fresh container to it once created.
	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{{Network: network.Network{ID: "net-1", Name: "p_db_relay"}}},
	}, nil)

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)

	var got client.ContainerCreateOptions
	apiMock.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			got = opts
			return client.ContainerCreateResult{ID: "relay-1"}, nil
		})
	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_db_relay", client.NetworkConnectOptions{
		Container: "relay-1",
	}).Return(client.NetworkConnectResult{}, nil)
	apiMock.EXPECT().ContainerStart(gomock.Any(), "relay-1", gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	endpoints := map[int]string{80: "host.docker.internal:49152"}
	err = svc.ensureServiceRelay(t.Context(), project, db, endpoints, []string{"default"})
	assert.NilError(t, err)

	assert.DeepEqual(t, got.HostConfig.CapDrop, []string{"ALL"})
	assert.DeepEqual(t, got.HostConfig.CapAdd, []string{"NET_BIND_SERVICE"})
}

// No relay ever existed for the service: nothing to do, and nothing calls
// ContainerRemove (mockCtrl.Finish would fail an unexpected call anyway).
func TestRemoveServiceRelayNoopWhenNoneExists(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{}, nil)
	// get-relay-info can create the link network speculatively before the
	// relay itself ever deploys: removeServiceRelay must still try to clean
	// it up even when no relay container ever existed.
	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)

	assert.NilError(t, svc.removeServiceRelay(t.Context(), "p", "db"))
}

// A concurrent up or down may have already removed the relay between
// findRelayContainer and ContainerRemove: the desired outcome (relay gone) is
// already achieved, so a not-found error must not fail the whole up.
func TestRemoveServiceRelayIgnoresNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{{ID: "relay-1", Names: []string{"/p-db-1"}}},
	}, nil)
	apiMock.EXPECT().ContainerRemove(gomock.Any(), "relay-1", client.ContainerRemoveOptions{Force: true}).
		Return(client.ContainerRemoveResult{}, errdefs.ErrNotFound.WithMessage("already removed"))
	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)

	assert.NilError(t, svc.removeServiceRelay(t.Context(), "p", "db"))
}

// The daemon is already removing the relay (StateRemoving): a concurrent
// ContainerRemove would fail with "removal already in progress", so
// removeServiceRelay must wait for it instead, mirroring ensureServiceRelay.
func TestRemoveServiceRelayWaitsWhenAlreadyRemoving(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiMock, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	svc := tested.(*composeService)

	first := apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{{ID: "relay-1", Names: []string{"/p-db-1"}, State: container.StateRemoving}},
	}, nil)
	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{}, nil).After(first)
	apiMock.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)

	assert.NilError(t, svc.removeServiceRelay(t.Context(), "p", "db"))
}
