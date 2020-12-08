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

package moby

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/api/containers"
)

func TestToRuntimeConfig(t *testing.T) {
	t.Parallel()
	m := &types.ContainerJSON{
		Config: &container.Config{
			Env:    []string{"FOO1=BAR1", "FOO2=BAR2"},
			Labels: map[string]string{"foo1": "bar1", "foo2": "bar2"},
		},
	}
	rc := ToRuntimeConfig(m)
	res := &containers.RuntimeConfig{
		Env:    map[string]string{"FOO1": "BAR1", "FOO2": "BAR2"},
		Labels: []string{"foo1=bar1", "foo2=bar2"},
	}
	assert.DeepEqual(t, rc, res)
}

func TestToHostConfig(t *testing.T) {
	t.Parallel()
	base := &types.ContainerJSONBase{
		HostConfig: &container.HostConfig{
			AutoRemove: true,
			RestartPolicy: container.RestartPolicy{
				Name: "",
			},
			Resources: container.Resources{
				NanoCPUs: 750000000,
				Memory:   512 * 1024 * 1024,
			},
		},
	}
	m := &types.ContainerJSON{
		Config: &container.Config{
			Env:    []string{"FOO1=BAR1", "FOO2=BAR2"},
			Labels: map[string]string{"foo1": "bar1", "foo2": "bar2"},
		},
		ContainerJSONBase: base,
	}
	hc := ToHostConfig(m)
	res := &containers.HostConfig{
		AutoRemove:    true,
		RestartPolicy: containers.RestartPolicyNone,
		CPULimit:      0.75,
		MemoryLimit:   512 * 1024 * 1024,
	}
	assert.DeepEqual(t, hc, res)
}

func TestFromRestartPolicyName(t *testing.T) {
	t.Parallel()
	moby := []string{"always", "on-failure", "no", ""}
	ours := []string{
		containers.RestartPolicyAny,
		containers.RestartPolicyOnFailure,
		containers.RestartPolicyNone,
		containers.RestartPolicyNone,
	}
	for i, p := range moby {
		assert.Equal(t, fromRestartPolicyName(p), ours[i])
	}
}

func TestToRestartPolicy(t *testing.T) {
	t.Parallel()
	ours := []string{containers.RestartPolicyAny, containers.RestartPolicyOnFailure, containers.RestartPolicyNone}
	moby := []container.RestartPolicy{
		{Name: "always"},
		{Name: "on-failure"},
		{Name: "no"},
	}
	for i, p := range ours {
		assert.Equal(t, ToRestartPolicy(p), moby[i])
	}
}
