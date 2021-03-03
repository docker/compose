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
			Protocol:  p.Protocol,
		})
	}

	projectVolumes, serviceConfigVolumes, err := GetRunVolumes(r.Volumes)
	if err != nil {
		return types.Project{}, err
	}

	retries := uint64(r.Healthcheck.Retries)
	project := types.Project{
		Name: r.ID,
		Services: []types.ServiceConfig{
			{
				Name:        r.ID,
				Image:       r.Image,
				Command:     r.Command,
				Ports:       ports,
				Labels:      r.Labels,
				Volumes:     serviceConfigVolumes,
				DomainName:  r.DomainName,
				Environment: toComposeEnvs(r.Environment),
				HealthCheck: &types.HealthCheckConfig{
					Test:        r.Healthcheck.Test,
					Timeout:     &r.Healthcheck.Timeout,
					Interval:    &r.Healthcheck.Interval,
					Retries:     &retries,
					StartPeriod: &r.Healthcheck.StartPeriod,
					Disable:     r.Healthcheck.Disable,
				},
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
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
	result := types.MappingWithEquals{}
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
