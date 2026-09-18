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
	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
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

	err = svc.ensureRelayNetworks(t.Context(), project, existing, service, []string{"frontend"})
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

	err = svc.ensureRelayNetworks(t.Context(), project, existing, service, []string{"backend", "frontend"})
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

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{Items: []container.Summary{existing}}, nil)
	apiMock.EXPECT().NetworkConnect(gomock.Any(), "p_frontend", client.NetworkConnectOptions{
		Container:      "relay-1",
		EndpointConfig: &network.EndpointSettings{Aliases: []string{"db"}},
	}).Return(client.NetworkConnectResult{}, nil)

	err = svc.ensureServiceRelay(t.Context(), project, db, endpoints, []string{"backend", "frontend"})
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

	apiMock.EXPECT().ContainerList(gomock.Any(), gomock.Any()).
		Return(client.ContainerListResult{}, nil)

	var got client.ContainerCreateOptions
	apiMock.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			got = opts
			return client.ContainerCreateResult{ID: "relay-1"}, nil
		})
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

	assert.NilError(t, svc.removeServiceRelay(t.Context(), "p", "db"))
}
