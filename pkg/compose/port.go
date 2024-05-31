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

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/compose/v2/pkg/api"

	moby "github.com/docker/docker/api/types"
)

func (s *composeService) Port(ctx context.Context, projectName string, service string, port uint16, options api.PortOptions) (api.PortPublishers, error) {
	projectName = strings.ToLower(projectName)
	container, err := s.getSpecifiedContainer(ctx, projectName, oneOffInclude, false, service, options.Index)
	if err != nil {
		return nil, err
	}

	if port != 0 {
		for _, p := range container.Ports {
			if p.PrivatePort == port && p.Type == options.Protocol {
				return api.PortPublishers{
					api.PortPublisher{
						URL:           p.IP,
						TargetPort:    p.PrivatePort,
						PublishedPort: p.PublicPort,
						Protocol:      p.Type,
					},
				}, nil
			}
		}
		return nil, portNotFoundError(options.Protocol, port, container)
	}

	res := make(api.PortPublishers, len(container.Ports))
	for idx, p := range container.Ports {
		res[idx] = api.PortPublisher{
			URL:           p.IP,
			TargetPort:    p.PrivatePort,
			PublishedPort: p.PublicPort,
			Protocol:      p.Type,
		}
	}

	return res, nil
}

func portNotFoundError(protocol string, port uint16, ctr moby.Container) error {
	formatPort := func(protocol string, port uint16) string {
		return fmt.Sprintf("%d/%s", port, protocol)
	}

	var containerPorts []string
	for _, p := range ctr.Ports {
		containerPorts = append(containerPorts, formatPort(p.Type, p.PublicPort))
	}

	name := strings.TrimPrefix(ctr.Names[0], "/")
	return fmt.Errorf("no port %s for container %s: %s", formatPort(protocol, port), name, strings.Join(containerPorts, ", "))
}
