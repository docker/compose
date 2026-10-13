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

// Package naming derives the names Compose uses for the Docker resources it manages.
package naming

import (
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"

	"github.com/docker/compose/v5/pkg/api"
)

// ContainerName returns the name of the replica of a service: the container_name
// the service declares, or the default <project>-<service>-<number>.
func ContainerName(projectName string, service types.ServiceConfig, number int) string {
	if service.ContainerName != "" {
		return service.ContainerName
	}
	return strings.Join([]string{projectName, service.Name, strconv.Itoa(number)}, api.Separator)
}

// CanonicalContainerName returns the name a container goes by, without the
// leading slash nor the link aliases.
func CanonicalContainerName(c container.Summary) string {
	if len(c.Names) == 0 {
		// corner case, sometime happens on removal. return short ID as a safeguard value
		return c.ID[:12]
	}
	// Names return container canonical name /foo  + link aliases /linked_by/foo
	for _, name := range c.Names {
		if strings.LastIndex(name, "/") == 0 {
			return name[1:]
		}
	}

	return strings.TrimPrefix(c.Names[0], "/")
}
