package convert

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-10-01/containerinstance"

	"github.com/docker/api/containers"
)

// ToPorts converts Azure container ports to api ports
func ToPorts(ipAddr *containerinstance.IPAddress, ports []containerinstance.ContainerPort) []containers.Port {
	var result []containers.Port

	for _, port := range ports {
		if port.Port == nil {
			continue
		}
		protocol := "tcp"
		if port.Protocol != "" {
			protocol = string(port.Protocol)
		}
		ip := ""
		if ipAddr != nil {
			ip = *ipAddr.IP
		}

		result = append(result, containers.Port{
			HostPort:      uint32(*port.Port),
			ContainerPort: uint32(*port.Port),
			HostIP:        ip,
			Protocol:      strings.ToLower(protocol),
		})
	}

	return result
}
