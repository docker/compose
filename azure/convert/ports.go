package convert

import (
	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"

	"github.com/docker/api/containers"
)

// ToPorts converts Azure container ports to api ports
func ToPorts(ports []containerinstance.ContainerPort) []containers.Port {
	var result []containers.Port

	for _, port := range ports {
		if port.Port == nil {
			continue
		}
		result = append(result, containers.Port{
			ContainerPort: uint32(*port.Port),
			Protocol:      "tcp",
		})
	}

	return result
}
