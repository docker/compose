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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/streams"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/docker/compose/v2/pkg/api"
)

// NewComposeService create a local implementation of the compose.Service API
func NewComposeService(dockerCli command.Cli) api.Service {
	return &composeService{
		dockerCli: dockerCli,
	}
}

type composeService struct {
	dockerCli command.Cli
}

func (s *composeService) apiClient() client.APIClient {
	return s.dockerCli.Client()
}

func (s *composeService) configFile() *configfile.ConfigFile {
	return s.dockerCli.ConfigFile()
}

func (s *composeService) stdout() *streams.Out {
	return s.dockerCli.Out()
}

func (s *composeService) stdin() *streams.In {
	return s.dockerCli.In()
}

func (s *composeService) stderr() io.Writer {
	return s.dockerCli.Err()
}

func getCanonicalContainerName(c moby.Container) string {
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
	return c.Names[0][1:]
}

func getContainerNameWithoutProject(c moby.Container) string {
	name := getCanonicalContainerName(c)
	project := c.Labels[api.ProjectLabel]
	prefix := fmt.Sprintf("%s_%s_", project, c.Labels[api.ServiceLabel])
	if strings.HasPrefix(name, prefix) {
		return name[len(project)+1:]
	}
	return name
}

func (s *composeService) Convert(ctx context.Context, project *types.Project, options api.ConvertOptions) ([]byte, error) {
	switch options.Format {
	case "json":
		return json.MarshalIndent(project, "", "  ")
	case "yaml":
		return yaml.Marshal(project)
	default:
		return nil, fmt.Errorf("unsupported format %q", options)
	}
}

// projectFromName builds a types.Project based on actual resources with compose labels set
func (s *composeService) projectFromName(containers Containers, projectName string, services ...string) (*types.Project, error) {
	project := &types.Project{
		Name: projectName,
	}
	if len(containers) == 0 {
		return project, errors.Wrap(api.ErrNotFound, fmt.Sprintf("no container found for project %q", projectName))
	}
	set := map[string]*types.ServiceConfig{}
	for _, c := range containers {
		serviceLabel := c.Labels[api.ServiceLabel]
		_, ok := set[serviceLabel]
		if !ok {
			set[serviceLabel] = &types.ServiceConfig{
				Name:   serviceLabel,
				Image:  c.Image,
				Labels: c.Labels,
			}
		}
		set[serviceLabel].Scale++
	}
	for _, service := range set {
		dependencies := service.Labels[api.DependenciesLabel]
		if len(dependencies) > 0 {
			service.DependsOn = types.DependsOnConfig{}
			for _, dc := range strings.Split(dependencies, ",") {
				dcArr := strings.Split(dc, ":")
				condition := ServiceConditionRunningOrHealthy
				dependency := dcArr[0]

				// backward compatibility
				if len(dcArr) > 1 {
					condition = dcArr[1]
				}
				service.DependsOn[dependency] = types.ServiceDependency{Condition: condition}
			}
		}
		project.Services = append(project.Services, *service)
	}
SERVICES:
	for _, qs := range services {
		for _, es := range project.Services {
			if es.Name == qs {
				continue SERVICES
			}
		}
		return project, errors.Wrapf(api.ErrNotFound, "no such service: %q", qs)
	}
	err := project.ForServices(services)
	if err != nil {
		return project, err
	}

	return project, nil
}

func (s *composeService) actualVolumes(ctx context.Context, projectName string) (types.Volumes, error) {
	volumes, err := s.apiClient().VolumeList(ctx, filters.NewArgs(projectFilter(projectName)))
	if err != nil {
		return nil, err
	}

	actual := types.Volumes{}
	for _, vol := range volumes.Volumes {
		actual[vol.Labels[api.VolumeLabel]] = types.VolumeConfig{
			Name:   vol.Name,
			Driver: vol.Driver,
			Labels: vol.Labels,
		}
	}
	return actual, nil
}

func (s *composeService) actualNetworks(ctx context.Context, projectName string) (types.Networks, error) {
	networks, err := s.apiClient().NetworkList(ctx, moby.NetworkListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	if err != nil {
		return nil, err
	}

	actual := types.Networks{}
	for _, net := range networks {
		actual[net.Labels[api.NetworkLabel]] = types.NetworkConfig{
			Name:   net.Name,
			Driver: net.Driver,
			Labels: net.Labels,
		}
	}
	return actual, nil
}
