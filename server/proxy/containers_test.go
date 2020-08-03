package proxy

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/api/containers"
	"github.com/docker/api/formatter"
	containersv1 "github.com/docker/api/protos/containers/v1"
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
	}

	cc := grpcContainerToContainerConfig(r)
	assert.Equal(t, cc.ID, "myId")
	assert.Equal(t, cc.Image, "myImage")
	assert.Equal(t, cc.MemLimit, formatter.MemBytes(41))
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
}
