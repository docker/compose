package convert

import (
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/api/containers"
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

	composeRestartPolicyCondition := r.RestartPolicyCondition
	if composeRestartPolicyCondition == "no" {
		composeRestartPolicyCondition = "none"
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
						Condition: composeRestartPolicyCondition,
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
		tokens := strings.Split(env, "=")
		if len(tokens) > 1 {
			result[tokens[0]] = &tokens[1]
		} else {
			result[env] = nil
		}
	}
	return result
}
