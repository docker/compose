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
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2019-12-01/containerinstance"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/containers"
)

func convertPortsToAci(service serviceConfigAciHelper) ([]containerinstance.ContainerPort, []containerinstance.Port, *string, error) {
	var groupPorts []containerinstance.Port
	var containerPorts []containerinstance.ContainerPort
	for _, portConfig := range service.Ports {
		if portConfig.Published != 0 && portConfig.Published != portConfig.Target {
			msg := fmt.Sprintf("Port mapping is not supported with ACI, cannot map port %d to %d for container %s",
				portConfig.Published, portConfig.Target, service.Name)
			return nil, nil, nil, errors.New(msg)
		}
		portNumber := int32(portConfig.Target)
		var groupProtocol containerinstance.ContainerGroupNetworkProtocol
		var containerProtocol containerinstance.ContainerNetworkProtocol
		switch portConfig.Protocol {
		case "tcp", "":
			groupProtocol = containerinstance.TCP
			containerProtocol = containerinstance.ContainerNetworkProtocolTCP
		case "udp":
			groupProtocol = containerinstance.UDP
			containerProtocol = containerinstance.ContainerNetworkProtocolUDP
		default:
			return nil, nil, nil, fmt.Errorf("unknown protocol %q in exposed port for service %q", portConfig.Protocol, service.Name)
		}
		containerPorts = append(containerPorts, containerinstance.ContainerPort{
			Port:     to.Int32Ptr(portNumber),
			Protocol: containerProtocol,
		})
		groupPorts = append(groupPorts, containerinstance.Port{
			Port:     to.Int32Ptr(portNumber),
			Protocol: groupProtocol,
		})
	}
	var dnsLabelName *string
	if service.DomainName != "" {
		dnsLabelName = &service.DomainName
	}
	return containerPorts, groupPorts, dnsLabelName, nil
}

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
		if ipAddr != nil && ipAddr.IP != nil {
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
