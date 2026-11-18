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

	"github.com/docker/compose/v5/internal/confighash"
)

func TestUmaskOption(t *testing.T) {
	zero, mask := uint32(0), uint32(0o77)

	t.Run("not set", func(t *testing.T) {
		got, err := umaskOption(types.ServiceConfig{}, "1.44")
		assert.NilError(t, err)
		assert.Assert(t, got == nil)
	})
	t.Run("set on a supporting engine", func(t *testing.T) {
		got, err := umaskOption(types.ServiceConfig{ContainerSpec: types.ContainerSpec{Umask: &mask}}, apiVersion156)
		assert.NilError(t, err)
		assert.DeepEqual(t, got, &mask)
	})
	t.Run("zero is a value, not an absence", func(t *testing.T) {
		got, err := umaskOption(types.ServiceConfig{ContainerSpec: types.ContainerSpec{Umask: &zero}}, "1.57")
		assert.NilError(t, err)
		assert.Assert(t, got != nil)
		assert.Equal(t, *got, uint32(0))
	})
	t.Run("set on an engine that would ignore it", func(t *testing.T) {
		_, err := umaskOption(types.ServiceConfig{ContainerSpec: types.ContainerSpec{Umask: &mask}}, "1.55")
		assert.ErrorContains(t, err, "umask requires Docker Engine v29.8 or later")
	})
	t.Run("unset is accepted by any engine", func(t *testing.T) {
		_, err := umaskOption(types.ServiceConfig{}, "1.24")
		assert.NilError(t, err)
	})
}

// The umask reaches the engine in the container's HostConfig, for services as
// well as for the pre_start containers, which are created the same way.
func TestGetCreateConfigsUmask(t *testing.T) {
	mask := uint32(0o27)
	project := &types.Project{Name: "demo"}
	service := types.ServiceConfig{
		Name:          "app",
		ContainerSpec: types.ContainerSpec{Image: "alpine", Umask: &mask},
	}

	t.Run("supported", func(t *testing.T) {
		tested, _ := newPreStartTestServiceWithVersion(t, apiVersion156)
		cfgs, err := tested.getCreateConfigs(t.Context(), project, service, 1, nil, createOptions{Labels: map[string]string{}})
		assert.NilError(t, err)
		assert.Assert(t, cfgs.Host.Umask != nil)
		assert.Equal(t, *cfgs.Host.Umask, uint32(0o27))
	})
	t.Run("not set", func(t *testing.T) {
		tested, _ := newPreStartTestServiceWithVersion(t, apiVersion156)
		service := service
		service.Umask = nil
		cfgs, err := tested.getCreateConfigs(t.Context(), project, service, 1, nil, createOptions{Labels: map[string]string{}})
		assert.NilError(t, err)
		assert.Assert(t, cfgs.Host.Umask == nil)
	})
	t.Run("unsupported engine", func(t *testing.T) {
		tested, _ := newPreStartTestServiceWithVersion(t, "1.55")
		_, err := tested.getCreateConfigs(t.Context(), project, service, 1, nil, createOptions{Labels: map[string]string{}})
		assert.ErrorContains(t, err, "umask requires Docker Engine v29.8 or later")
	})
}

func TestServiceHashTracksUmask(t *testing.T) {
	zero, mask := uint32(0), uint32(0o77)
	hash := func(umask *uint32) string {
		h, err := confighash.ServiceHash(types.ServiceConfig{Name: "app", ContainerSpec: types.ContainerSpec{Image: "alpine", Umask: umask}})
		assert.NilError(t, err)
		return h
	}
	assert.Assert(t, hash(nil) != hash(&zero), "an explicit 0 differs from no umask")
	assert.Assert(t, hash(&zero) != hash(&mask))
}
