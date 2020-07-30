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
