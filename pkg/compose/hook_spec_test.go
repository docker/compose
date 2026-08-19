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
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func strPtr(s string) *string { return &s }

func TestMergedPreStartSpec(t *testing.T) {
	service := types.ServiceConfig{
		Name: "db",
		ContainerSpec: types.ContainerSpec{
			Image:       "postgres:16",
			User:        "999",
			WorkingDir:  "/srv",
			Command:     types.ShellCommand{"postgres"},
			Environment: types.MappingWithEquals{"PGDATA": strPtr("/data"), "SHARED": strPtr("service")},
			ExtraHosts:  types.HostsList{"inherited": {"192.0.2.10"}, "shared": {"192.0.2.11"}},
			DNS:         types.StringList{"1.1.1.1"},
			CapAdd:      []string{"NET_ADMIN"},
			Volumes:     []types.ServiceVolumeConfig{{Type: types.VolumeTypeVolume, Source: "data", Target: "/data", ReadOnly: true}},
		},
	}
	hook := types.PreStartHook{ContainerSpec: types.ContainerSpec{
		Command:     types.ShellCommand{"init.sh"},
		Environment: types.MappingWithEquals{"SHARED": strPtr("hook"), "ONLY": strPtr("hook")},
		ExtraHosts:  types.HostsList{"declared": {"192.0.2.20"}, "shared": {"192.0.2.99"}},
		DNS:         types.StringList{"8.8.8.8"},
		Volumes:     []types.ServiceVolumeConfig{{Type: types.VolumeTypeVolume, Source: "data", Target: "/data"}},
	}}

	spec, err := mergedPreStartSpec(service, hook)
	assert.NilError(t, err)

	// scalars inherit when undeclared
	assert.Check(t, is.Equal(spec.Image, "postgres:16"))
	assert.Check(t, is.Equal(spec.User, "999"))
	assert.Check(t, is.Equal(spec.WorkingDir, "/srv"))
	// command replaces
	assert.Check(t, is.DeepEqual([]string(spec.Command), []string{"init.sh"}))
	// collections merge per the compose file rules: environment per key
	// (hook wins), extra_hosts accumulate entries per hostname
	assert.Check(t, is.Equal(*spec.Environment["PGDATA"], "/data"))
	assert.Check(t, is.Equal(*spec.Environment["SHARED"], "hook"))
	assert.Check(t, is.Equal(*spec.Environment["ONLY"], "hook"))
	assert.Check(t, is.DeepEqual(spec.ExtraHosts["inherited"], []string{"192.0.2.10"}))
	assert.Check(t, is.DeepEqual(spec.ExtraHosts["declared"], []string{"192.0.2.20"}))
	assert.Check(t, is.DeepEqual(spec.ExtraHosts["shared"], []string{"192.0.2.11", "192.0.2.99"}))
	assert.Check(t, is.DeepEqual([]string(spec.DNS), []string{"1.1.1.1", "8.8.8.8"}))
	// previously-unwired attributes inherit too, with zero dedicated code
	assert.Check(t, is.DeepEqual(spec.CapAdd, []string{"NET_ADMIN"}))
	// volumes are NOT inherited at the model level: runtime volumes_from does
	assert.Assert(t, is.Len(spec.Volumes, 1))
	assert.Check(t, !spec.Volumes[0].ReadOnly, "hook keeps its own rw declaration")
}

func TestMergedPreStartSpecEmptyHook(t *testing.T) {
	service := types.ServiceConfig{ContainerSpec: types.ContainerSpec{
		Image: "alpine", Sysctls: types.Mapping{"net.core.somaxconn": "1024"},
	}}
	spec, err := mergedPreStartSpec(service, types.PreStartHook{})
	assert.NilError(t, err)
	assert.Check(t, is.Equal(spec.Image, "alpine"))
	assert.Check(t, is.Equal(spec.Sysctls["net.core.somaxconn"], "1024"))
	assert.Check(t, is.Len(spec.Volumes, 0))
}
