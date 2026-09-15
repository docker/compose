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
	"github.com/moby/moby/api/types/container"
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
				Name:      "app",
				DependsOn: types.DependsOnConfig{"db": {}},
				Networks:  map[string]*types.ServiceNetworkConfig{"backend": nil},
			},
			"web": {
				Name:      "web",
				DependsOn: types.DependsOnConfig{"db": {}},
				Networks:  map[string]*types.ServiceNetworkConfig{"frontend": nil, "backend": nil},
			},
			"other": {
				Name:     "other",
				Networks: map[string]*types.ServiceNetworkConfig{"private": nil},
			},
		},
		Networks: types.Networks{"default": {}, "backend": {}, "frontend": {}, "private": {}},
	}

	assert.DeepEqual(t, relayNetworks(project, db), []string{"backend", "frontend"})

	// no consumer: fall back to the project default network
	lonely := types.ServiceConfig{Name: "lonely", Provider: &types.ServiceProviderConfig{Type: "test"}}
	assert.DeepEqual(t, relayNetworks(project, lonely), []string{"default"})
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
