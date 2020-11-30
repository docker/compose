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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	types2 "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/errdefs"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"github.com/sanathkr/go-yaml"
	"golang.org/x/mod/semver"
)

func (e ecsLocalSimulation) Build(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (e ecsLocalSimulation) Up(ctx context.Context, project *types.Project, detach bool) error {
	cmd := exec.Command("docker-compose", "version", "--short")
	b := bytes.Buffer{}
	b.WriteString("v")
	cmd.Stdout = bufio.NewWriter(&b)
	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "ECS simulation mode require Docker-compose 1.27")
	}
	version := semver.MajorMinor(strings.TrimSpace(b.String()))
	if version == "" {
		return fmt.Errorf("can't parse docker-compose version: %s", b.String())
	}
	if semver.Compare(version, "v1.27") < 0 {
		return fmt.Errorf("ECS simulation mode require Docker-compose 1.27, found %s", version)
	}

	converted, err := e.Convert(ctx, project, "json")
	if err != nil {
		return err
	}

	cmd = exec.Command("docker-compose", "--context", "default", "--project-directory", project.WorkingDir, "--project-name", project.Name, "-f", "-", "up")
	cmd.Stdin = strings.NewReader(string(converted))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e ecsLocalSimulation) Convert(ctx context.Context, project *types.Project, format string) ([]byte, error) {
	project.Networks["credentials_network"] = types.NetworkConfig{
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

	delete(project.Networks, "default")
	config := map[string]interface{}{
		"services": project.Services,
		"networks": project.Networks,
		"volumes":  project.Volumes,
		"secrets":  project.Secrets,
		"configs":  project.Configs,
	}
	switch format {
	case "json":
		return json.MarshalIndent(config, "", "  ")
	case "yaml":
		return yaml.Marshal(config)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}

}

func (e ecsLocalSimulation) Down(ctx context.Context, projectName string) error {
	cmd := exec.Command("docker-compose", "--context", "default", "--project-name", projectName, "-f", "-", "down", "--remove-orphans")
	cmd.Stdin = strings.NewReader(string(`
services:
   ecs-local-endpoints:
      image: "amazon/amazon-ecs-local-container-endpoints"
`))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e ecsLocalSimulation) Logs(ctx context.Context, projectName string, w io.Writer) error {
	list, err := e.moby.ContainerList(ctx, types2.ContainerListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName)),
	})
	if err != nil {
		return err
	}
	services := map[string]types.ServiceConfig{}
	for _, c := range list {
		services[c.Labels["com.docker.compose.service"]] = types.ServiceConfig{
			Image: "unused",
		}
	}

	marshal, err := yaml.Marshal(map[string]interface{}{
		"services": services,
	})
	if err != nil {
		return err
	}
	cmd := exec.Command("docker-compose", "--context", "default", "--project-name", projectName, "-f", "-", "logs", "-f")
	cmd.Stdin = strings.NewReader(string(marshal))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e ecsLocalSimulation) Ps(ctx context.Context, projectName string) ([]compose.ServiceStatus, error) {
	return nil, errors.Wrap(errdefs.ErrNotImplemented, "use docker-compose ps")
}
func (e ecsLocalSimulation) List(ctx context.Context, projectName string) ([]compose.Stack, error) {
	return nil, errors.Wrap(errdefs.ErrNotImplemented, "use docker-compose ls")
}
