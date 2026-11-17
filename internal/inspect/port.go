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

package inspect

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/pkg/api"
)

func (s *Service) Ports(ctx context.Context, projectName string, serviceName string, port uint16, options api.PortOptions) (api.PortPublishers, error) {
	projectName = strings.ToLower(projectName)
	ctr, err := s.backend.SpecifiedContainer(ctx, projectName, engine.OneOffInclude, false, serviceName, options.Index)
	if err != nil {
		return nil, err
	}

	// the single definition of "published": exposed by the image but not
	// published to the host, a port has nothing to report, like `docker port`
	var published api.PortPublishers
	for _, p := range containerPublishers(ctr) {
		if p.PublishedPort != 0 {
			published = append(published, p)
		}
	}

	var publishers api.PortPublishers
	for _, p := range published {
		if options.Protocol != "" && p.Protocol != options.Protocol {
			continue
		}
		if port != 0 && p.TargetPort != int(port) {
			continue
		}
		publishers = append(publishers, p)
	}
	if port != 0 && len(publishers) == 0 {
		return nil, portNotFoundError(options.Protocol, port, ctr, published)
	}
	return publishers, nil
}

func portNotFoundError(protocol string, port uint16, ctr container.Summary, published api.PortPublishers) error {
	formatPort := func(protocol string, port int) string {
		return fmt.Sprintf("%d/%s", port, protocol)
	}

	containerPorts := make([]string, 0, len(published))
	for _, p := range published {
		containerPorts = append(containerPorts, formatPort(p.Protocol, p.TargetPort))
	}

	name := strings.TrimPrefix(ctr.Names[0], "/")
	if len(containerPorts) == 0 {
		return fmt.Errorf("no port %s for container %s", formatPort(protocol, int(port)), name)
	}
	return fmt.Errorf("no port %s for container %s: %s", formatPort(protocol, int(port)), name, strings.Join(containerPorts, ", "))
}
