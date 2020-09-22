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

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/api/containers"
)

// ContainerToComposeProject convert container config to compose project
func ContainerToComposeProject(r containers.ContainerConfig) (types.Project, error) {
	var ports []types.ServicePortConfig
	for _, p := range r.Ports {
		ports = append(ports, types.ServicePortConfig{
			Target:    p.ContainerPort,
			Published: p.HostPort,
		})
	}

	projectVolumes, serviceConfigVolumes, err := GetRunVolumes(r.Volumes)
	if err != nil {
		return types.Project{}, err
	}

	project := types.Project{
		Name: r.ID,
		Services: []types.ServiceConfig{
			{
				Name:        r.ID,
				Image:       r.Image,
				Ports:       ports,
				Labels:      r.Labels,
				Volumes:     serviceConfigVolumes,
				Environment: toComposeEnvs(r.Environment),
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Limits: &types.Resource{
							NanoCPUs:    fmt.Sprintf("%f", r.CPULimit),
							MemoryBytes: types.UnitBytes(r.MemLimit.Value()),
						},
					},
					RestartPolicy: &types.RestartPolicy{
						Condition: r.RestartPolicyCondition,
					},
				},
			},
		},
		Volumes: projectVolumes,
	}
	return project, nil
}

func toComposeEnvs(opts []string) types.MappingWithEquals {
	result := map[string]*string{}
	for _, env := range opts {
		tokens := strings.SplitN(env, "=", 2)
		if len(tokens) > 1 {
			result[tokens[0]] = &tokens[1]
		} else {
			result[env] = nil
		}
	}
	return result
}
