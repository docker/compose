/*
   Copyright 2020 Docker, Inc.

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
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/containers"
)

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
