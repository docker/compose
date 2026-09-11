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
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"
)

func TestServiceHash(t *testing.T) {
	hash1, err := ServiceHash(serviceConfig(1))
	assert.NilError(t, err)
	hash2, err := ServiceHash(serviceConfig(2))
	assert.NilError(t, err)
	assert.Equal(t, hash1, hash2)
}

func serviceConfig(replicas int) types.ServiceConfig {
	return types.ServiceConfig{
		Scale: &replicas,
		Deploy: &types.DeployConfig{
			Replicas: &replicas,
		},
		Name:  "foo",
		Image: "bar",
	}
}

// TestServiceHashContinuity proves the pinned serializer reproduces the
// historical bytes: while compose-go's struct order still matches the frozen
// list — true on this branch's compose-go — the pinned hash and the plain
// struct-marshal hash are byte-identical. The compose-go upgrade that first
// reorders the struct (the container-spec layering) deletes this test in the
// same commit: from that point the frozen list carries continuity alone,
// locked by TestHashGoldenValues.
func TestServiceHashContinuity(t *testing.T) {
	svc := richServiceFixture()
	pinned, err := ServiceHash(svc)
	assert.NilError(t, err)

	o := svc
	o.Build = nil
	o.PullPolicy = ""
	o.Scale = nil
	if o.Deploy != nil {
		deploy := *o.Deploy
		deploy.Replicas = nil
		o.Deploy = &deploy
	}
	o.DependsOn = nil
	o.Profiles = nil
	raw, err := json.Marshal(o)
	assert.NilError(t, err)
	legacy := digest.SHA256.FromBytes(raw).Encoded()
	assert.Equal(t, pinned, legacy)
	t.Logf("GOLDEN service=%s", pinned)
}

func richServiceFixture() types.ServiceConfig {
	replicas := 3
	return types.ServiceConfig{
		Name:        "web",
		Image:       "nginx:latest",
		Command:     types.ShellCommand{"nginx", "-g", "daemon off;"},
		User:        "nobody",
		Tty:         true,
		StdinOpen:   true,
		Restart:     types.RestartPolicyAlways,
		Environment: types.MappingWithEquals{"A": strPtr("1"), "B": nil},
		Labels:      types.Labels{"com.example": "v"},
		Annotations: types.Mapping{"note": "x"},
		CapAdd:      []string{"NET_ADMIN"},
		ExtraHosts:  types.HostsList{"alpha": []string{"10.0.0.1"}},
		Deploy:      &types.DeployConfig{Replicas: &replicas},
		Ports: []types.ServicePortConfig{
			{Target: 80, Published: "8080", Protocol: "tcp"},
		},
		HealthCheck: &types.HealthCheckConfig{
			Test: types.HealthCheckTest{"CMD", "true"},
		},
		Volumes: []types.ServiceVolumeConfig{
			{Type: types.VolumeTypeVolume, Source: "data", Target: "/data"},
		},
		Networks: map[string]*types.ServiceNetworkConfig{"default": nil},
	}
}

func strPtr(s string) *string { return &s }

func TestHashGoldenValues(t *testing.T) {
	svc, err := ServiceHash(richServiceFixture())
	assert.NilError(t, err)
	assert.Equal(t, svc, "75bc312132c71fac4971d123202631b6978ac9d796336621af744d46611b42a2")

	nw, err := NetworkHash(&types.NetworkConfig{Name: "proj_default", Driver: "bridge"})
	assert.NilError(t, err)
	assert.Equal(t, nw, "eeef29d8955b1d4e9382986e213c80789065d989d7a6d035163e0180159aaac0")

	vol, err := VolumeHash(types.VolumeConfig{Name: "proj_data"})
	assert.NilError(t, err)
	assert.Equal(t, vol, "dd3953f0ff20e0f9044086b0483690f2cc2b4653e57aaaa5fa8ea2735da61000")
}

// pinRootKeyOrder is the identity for an object whose keys already follow
// the frozen order — the property that keeps every released hash valid.
func TestPinRootKeyOrderIdentity(t *testing.T) {
	in := []byte(`{"profiles":["p"],"command":["c"],"image":"img","user":"u"}`)
	out, err := pinRootKeyOrder(in, serviceHashKeyOrder)
	assert.NilError(t, err)
	assert.Equal(t, string(out), string(in))
}

// The pinned form is a function of the configuration VALUES, not of the
// struct layout it travels in: two types declaring the same JSON fields in a
// different order digest identically.
func TestPinRootKeyOrderIgnoresFieldOrder(t *testing.T) {
	type a struct {
		Image string   `json:"image"`
		Ports []string `json:"ports,omitempty"`
		User  string   `json:"user,omitempty"`
	}
	type b struct {
		User  string   `json:"user,omitempty"`
		Image string   `json:"image"`
		Ports []string `json:"ports,omitempty"`
	}
	ra, _ := json.Marshal(a{Image: "nginx", Ports: []string{"80:80"}, User: "nobody"})
	rb, _ := json.Marshal(b{Image: "nginx", Ports: []string{"80:80"}, User: "nobody"})
	pa, err := pinRootKeyOrder(ra, serviceHashKeyOrder)
	assert.NilError(t, err)
	pb, err := pinRootKeyOrder(rb, serviceHashKeyOrder)
	assert.NilError(t, err)
	assert.Equal(t, string(pa), string(pb))
}

// Keys unknown to the frozen list are appended in sorted order, each emitted
// exactly once: nothing an attribute addition brings can be silently dropped.
func TestPinRootKeyOrderAppendsUnknownSorted(t *testing.T) {
	in := []byte(`{"zz_new":"2","image":"img","aa_new":"1"}`)
	out, err := pinRootKeyOrder(in, serviceHashKeyOrder)
	assert.NilError(t, err)
	assert.Equal(t, string(out), `{"image":"img","aa_new":"1","zz_new":"2"}`)
}

// Every root JSON key of compose-go's ServiceConfig must be in the frozen
// list: a compose-go upgrade adding an attribute fails here, so extending the
// hash surface is a reviewed decision — add the new key at the END of
// serviceHashKeyOrder (its hash only moves for configurations using it).
func TestServiceHashKeyOrderCoversStruct(t *testing.T) {
	known := map[string]bool{}
	for _, k := range serviceHashKeyOrder {
		known[k] = true
	}
	var walk func(t reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			name, _, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				// encoding/json dereferences embedded pointers; match it
				ft := f.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				walk(ft)
				continue
			}
			if name == "" {
				name = f.Name
			}
			assert.Assert(t, known[name],
				"ServiceConfig root attribute %q is not in serviceHashKeyOrder: append it at the end of the list (a reviewed decision — the hash of configurations using it will move)", name)
		}
	}
	walk(reflect.TypeOf(types.ServiceConfig{}))
}
