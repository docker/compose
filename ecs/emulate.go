/*
   Copyright 2020 Docker, Inc.

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

package ecs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/compose-spec/compose-go/types"
	"github.com/sanathkr/go-yaml"

	"github.com/compose-spec/compose-go/cli"
)

func (c *ecsAPIService) Emulate(ctx context.Context, options *cli.ProjectOptions) error {
	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return err
	}
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
		return err
	}

	for i, service := range project.Services {
		service.Networks["credentials_network"] = &types.ServiceNetworkConfig{
			Ipv4Address: fmt.Sprintf("169.254.170.%d", i+3),
		}
		service.DependsOn = append(service.DependsOn, "ecs-local-endpoints")
		service.Environment["AWS_DEFAULT_REGION"] = aws.String(c.ctx.Region)
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
	marshal, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	cmd := exec.Command("docker-compose", "--context", "default", "--project-directory", project.WorkingDir, "--project-name", project.Name, "-f", "-", "up")
	cmd.Stdin = strings.NewReader(string(marshal))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
