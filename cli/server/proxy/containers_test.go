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

package proxy

import (
	"testing"

	"github.com/docker/compose-cli/utils"

	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/api/containers"
	containersv1 "github.com/docker/compose-cli/cli/server/protos/containers/v1"
)

func TestGrpcContainerToContainerConfig(t *testing.T) {
	r := &containersv1.RunRequest{
		Id:    "myId",
		Image: "myImage",
		Ports: []*containersv1.Port{
			{
				HostPort:      8080,
				ContainerPort: 80,
				Protocol:      "tcp",
				HostIp:        "42.42.42.42",
			},
		},
		Labels: map[string]string{
			"mykey": "mylabel",
		},
		Volumes: []string{
			"myvolume",
		},
		MemoryLimit: 41,
		CpuLimit:    42,
		Environment: []string{"PROTOVAR=VALUE"},
	}

	cc, err := grpcContainerToContainerConfig(r)
	assert.NilError(t, err)
	assert.Equal(t, cc.ID, "myId")
	assert.Equal(t, cc.Image, "myImage")
	assert.Equal(t, cc.MemLimit, utils.MemBytes(41))
	assert.Equal(t, cc.CPULimit, float64(42))
	assert.DeepEqual(t, cc.Volumes, []string{"myvolume"})
	assert.DeepEqual(t, cc.Ports, []containers.Port{
		{
			HostPort:      uint32(8080),
			ContainerPort: 80,
			Protocol:      "tcp",
			HostIP:        "42.42.42.42",
		},
	})
	assert.DeepEqual(t, cc.Labels, map[string]string{
		"mykey": "mylabel",
	})
	assert.DeepEqual(t, cc.Environment, []string{"PROTOVAR=VALUE"})
}
