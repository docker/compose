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

	"github.com/docker/compose/v2/pkg/api"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) Port(ctx context.Context, project string, service string, port int, options api.PortOptions) (string, int, error) {
	list, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project),
			serviceFilter(service),
			containerNumberFilter(options.Index),
		),
	})
	if err != nil {
		return "", 0, err
	}
	if len(list) == 0 {
		return "", 0, fmt.Errorf("no container found for %s_%d", service, options.Index)
	}
	container := list[0]
	for _, p := range container.Ports {
		if p.PrivatePort == uint16(port) && p.Type == options.Protocol {
			return p.IP, int(p.PublicPort), nil
		}
	}
	return "", 0, err
}
