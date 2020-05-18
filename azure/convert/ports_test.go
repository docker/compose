package convert

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerinstance/mgmt/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/stretchr/testify/assert"

	"github.com/docker/api/containers"
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
			assert.Equal(t, testCase.expected, ports)
		})
	}
}
