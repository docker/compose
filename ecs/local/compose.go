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

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/sanathkr/go-yaml"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/errdefs"
)

func (e ecsLocalSimulation) Build(ctx context.Context, project *types.Project) error {
	return e.compose.Build(ctx, project)
}

func (e ecsLocalSimulation) Push(ctx context.Context, project *types.Project) error {
	return e.compose.Push(ctx, project)
}

func (e ecsLocalSimulation) Pull(ctx context.Context, project *types.Project) error {
	return e.compose.Pull(ctx, project)
}

func (e ecsLocalSimulation) Create(ctx context.Context, project *types.Project, opts compose.CreateOptions) error {
	enhanced, err := e.enhanceForLocalSimulation(project)
	if err != nil {
		return err
	}

	return e.compose.Create(ctx, enhanced, opts)
}

func (e ecsLocalSimulation) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	return e.compose.Start(ctx, project, options)
}

func (e ecsLocalSimulation) Stop(ctx context.Context, project *types.Project, options compose.StopOptions) error {
	return e.compose.Stop(ctx, project, options)
}

func (e ecsLocalSimulation) Up(ctx context.Context, project *types.Project, options compose.UpOptions) error {
	return errdefs.ErrNotImplemented
}

func (e ecsLocalSimulation) Kill(ctx context.Context, project *types.Project, options compose.KillOptions) error {
	return e.compose.Kill(ctx, project, options)
}

func (e ecsLocalSimulation) Convert(ctx context.Context, project *types.Project, options compose.ConvertOptions) ([]byte, error) {
	enhanced, err := e.enhanceForLocalSimulation(project)
	if err != nil {
		return nil, err
	}

	delete(enhanced.Networks, "default")
	config := map[string]interface{}{
		"services": enhanced.Services,
		"networks": enhanced.Networks,
		"volumes":  enhanced.Volumes,
		"secrets":  enhanced.Secrets,
		"configs":  enhanced.Configs,
	}
	switch options.Format {
	case "json":
		return json.MarshalIndent(config, "", "  ")
	case "yaml":
		return yaml.Marshal(config)
	default:
		return nil, fmt.Errorf("unsupported format %q", options)
	}

}

func (e ecsLocalSimulation) enhanceForLocalSimulation(project *types.Project) (*types.Project, error) {
	project.Networks["credentials_network"] = types.NetworkConfig{
		Name:   "credentials_network",
		Driver: "bridge",
		Ipam: types.IPAMConfig{
			Config: []*types.IPAMPool{
				{
					Subnet:  "169.254.170.0/24",
					Gateway: "169.254.170.1",
				},
			},
		},
	}

	// On Windows, this directory can be found at "%UserProfile%\.aws"
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	for i, service := range project.Services {
		service.Networks["credentials_network"] = &types.ServiceNetworkConfig{
			Ipv4Address: fmt.Sprintf("169.254.170.%d", i+3),
		}
		if service.DependsOn == nil {
			service.DependsOn = types.DependsOnConfig{}
		}
		service.DependsOn["ecs-local-endpoints"] = types.ServiceDependency{
			Condition: types.ServiceConditionStarted,
		}
		service.Environment["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"] = aws.String("/creds")
		service.Environment["ECS_CONTAINER_METADATA_URI"] = aws.String("http://169.254.170.2/v3")
		project.Services[i] = service
	}

	project.Services = append(project.Services, types.ServiceConfig{
		Name:  "ecs-local-endpoints",
		Image: "amazon/amazon-ecs-local-container-endpoints",
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   types.VolumeTypeBind,
				Source: "/var/run",
				Target: "/var/run",
			},
			{
				Type:   types.VolumeTypeBind,
				Source: filepath.Join(home, ".aws"),
				Target: "/home/.aws",
			},
		},
		Environment: map[string]*string{
			"HOME":        aws.String("/home"),
			"AWS_PROFILE": aws.String("default"),
		},
		Networks: map[string]*types.ServiceNetworkConfig{
			"credentials_network": {
				Ipv4Address: "169.254.170.2",
			},
		},
	})
	return project, nil
}

func (e ecsLocalSimulation) Down(ctx context.Context, projectName string, options compose.DownOptions) error {
	options.RemoveOrphans = true
	return e.compose.Down(ctx, projectName, options)
}

func (e ecsLocalSimulation) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	return e.compose.Logs(ctx, projectName, consumer, options)
}

func (e ecsLocalSimulation) Ps(ctx context.Context, projectName string, options compose.PsOptions) ([]compose.ContainerSummary, error) {
	return e.compose.Ps(ctx, projectName, options)
}
func (e ecsLocalSimulation) List(ctx context.Context) ([]compose.Stack, error) {
	return e.compose.List(ctx)
}
func (e ecsLocalSimulation) RunOneOffContainer(ctx context.Context, project *types.Project, opts compose.RunOptions) (int, error) {
	return 0, errors.Wrap(errdefs.ErrNotImplemented, "use docker-compose run")
}

func (e ecsLocalSimulation) Remove(ctx context.Context, project *types.Project, options compose.RemoveOptions) ([]string, error) {
	return e.compose.Remove(ctx, project, options)
}
