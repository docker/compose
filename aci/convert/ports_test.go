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

package convert

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/compose-spec/compose-go/types"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"

	"github.com/docker/compose-cli/api/containers"
)

func TestComposeContainerGroupToContainerMultiplePorts(t *testing.T) {
	project := types.Project{
		Services: []types.ServiceConfig{
			{
				Name:  "service1",
				Image: "image1",
				Ports: []types.ServicePortConfig{
					{
						Published: 80,
						Target:    80,
					},
				},
			},
			{
				Name:  "service2",
				Image: "image2",
				Ports: []types.ServicePortConfig{
					{
						Published: 8080,
						Target:    8080,
					},
				},
			},
		},
	}

	group, err := ToContainerGroup(context.TODO(), convertCtx, project, mockStorageHelper)
	assert.NilError(t, err)
	assert.Assert(t, is.Len(*group.Containers, 3))

	container1 := (*group.Containers)[0]
	assert.Equal(t, *container1.Name, "service1")
	assert.Equal(t, *container1.Image, "image1")
	assert.Equal(t, *(*container1.Ports)[0].Port, int32(80))

	container2 := (*group.Containers)[1]
	assert.Equal(t, *container2.Name, "service2")
	assert.Equal(t, *container2.Image, "image2")
	assert.Equal(t, *(*container2.Ports)[0].Port, int32(8080))

	groupPorts := *group.IPAddress.Ports
	assert.Assert(t, is.Len(groupPorts, 2))
	assert.Equal(t, *groupPorts[0].Port, int32(80))
	assert.Equal(t, *groupPorts[1].Port, int32(8080))
	assert.Assert(t, group.IPAddress.DNSNameLabel == nil)
}

func TestPortConvert(t *testing.T) {
	expectedPorts := []containers.Port{
		{
			HostPort:      80,
			ContainerPort: 80,
			HostIP:        "10.0.0.1",
			Protocol:      "tcp",
		},
	}
	testCases := []struct {
		name     string
		ip       *containerinstance.IPAddress
		ports    []containerinstance.ContainerPort
		expected []containers.Port
	}{
		{
			name: "convert port",
			ip: &containerinstance.IPAddress{
				IP: to.StringPtr("10.0.0.1"),
			},
			ports: []containerinstance.ContainerPort{
				{
					Protocol: "tcp",
					Port:     to.Int32Ptr(80),
				},
			},
			expected: expectedPorts,
		},
		{
			name: "with nil ip",
			ip:   nil,
			ports: []containerinstance.ContainerPort{
				{
					Protocol: "tcp",
					Port:     to.Int32Ptr(80),
				},
			},
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					HostIP:        "",
					Protocol:      "tcp",
				},
			},
		},
		{
			name: "with nil ip value",
			ip: &containerinstance.IPAddress{
				IP: nil,
			},
			ports: []containerinstance.ContainerPort{
				{
					Protocol: "tcp",
					Port:     to.Int32Ptr(80),
				},
			},
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					HostIP:        "",
					Protocol:      "tcp",
				},
			},
		},
		{
			name: "skip nil ports",
			ip:   nil,
			ports: []containerinstance.ContainerPort{
				{
					Protocol: "tcp",
					Port:     to.Int32Ptr(80),
				},
				{
					Protocol: "tcp",
					Port:     nil,
				},
			},
			expected: []containers.Port{
				{
					HostPort:      80,
					ContainerPort: 80,
					HostIP:        "",
					Protocol:      "tcp",
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ports := ToPorts(testCase.ip, testCase.ports)
			assert.DeepEqual(t, testCase.expected, ports)
		})
	}
}

func TestConvertTCPPortsToAci(t *testing.T) {
	service := types.ServiceConfig{
		Name: "myService",
		Ports: []types.ServicePortConfig{
			{
				Protocol:  "",
				Target:    80,
				Published: 80,
			},
			{
				Protocol:  "tcp",
				Target:    90,
				Published: 90,
			},
		},
	}
	containerPorts, groupPports, _, err := convertPortsToAci(serviceConfigAciHelper(service))
	assert.NilError(t, err)
	assert.DeepEqual(t, containerPorts, []containerinstance.ContainerPort{
		{
			Port:     to.Int32Ptr(80),
			Protocol: containerinstance.ContainerNetworkProtocolTCP,
		},
		{
			Port:     to.Int32Ptr(90),
			Protocol: containerinstance.ContainerNetworkProtocolTCP,
		},
	})
	assert.DeepEqual(t, groupPports, []containerinstance.Port{
		{
			Port:     to.Int32Ptr(80),
			Protocol: containerinstance.TCP,
		},
		{
			Port:     to.Int32Ptr(90),
			Protocol: containerinstance.TCP,
		},
	})
}

func TestConvertUDPPortsToAci(t *testing.T) {
	service := types.ServiceConfig{
		Name: "myService",
		Ports: []types.ServicePortConfig{
			{
				Protocol:  "udp",
				Target:    80,
				Published: 80,
			},
		},
	}
	containerPorts, groupPports, _, err := convertPortsToAci(serviceConfigAciHelper(service))
	assert.NilError(t, err)
	assert.DeepEqual(t, containerPorts, []containerinstance.ContainerPort{
		{
			Port:     to.Int32Ptr(80),
			Protocol: containerinstance.ContainerNetworkProtocolUDP,
		},
	})
	assert.DeepEqual(t, groupPports, []containerinstance.Port{
		{
			Port:     to.Int32Ptr(80),
			Protocol: containerinstance.UDP,
		},
	})
}

func TestConvertErrorOnMappingPorts(t *testing.T) {
	service := types.ServiceConfig{
		Name: "myService",
		Ports: []types.ServicePortConfig{
			{
				Protocol:  "",
				Target:    80,
				Published: 90,
			},
		},
	}
	_, _, _, err := convertPortsToAci(serviceConfigAciHelper(service))
	assert.Error(t, err, "Port mapping is not supported with ACI, cannot map port 90 to 80 for container myService")
}
