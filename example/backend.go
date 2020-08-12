// +build example

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

package example

import (
	"context"
	"errors"
	"fmt"

	"github.com/compose-spec/compose-go/cli"
	ecstypes "github.com/docker/ecs-plugin/pkg/compose"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	"github.com/docker/api/context/cloud"
	"github.com/docker/api/errdefs"
)

type apiService struct {
	containerService
	composeService
}

func (a *apiService) ContainerService() containers.Service {
	return &a.containerService
}

func (a *apiService) ComposeService() compose.Service {
	return &a.composeService
}

func init() {
	backend.Register("example", "example", service, cloud.NotImplementedCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	return &apiService{}, nil
}

type containerService struct{}

func (cs *containerService) Inspect(ctx context.Context, id string) (containers.Container, error) {
	return containers.Container{
		ID:                     "id",
		Image:                  "nginx",
		Platform:               "Linux",
		RestartPolicyCondition: "none",
	}, nil
}

func (cs *containerService) List(ctx context.Context, all bool) ([]containers.Container, error) {
	result := []containers.Container{
		{
			ID:    "id",
			Image: "nginx",
		},
		{
			ID:    "1234",
			Image: "alpine",
		},
	}

	if all {
		result = append(result, containers.Container{
			ID:    "stopped",
			Image: "nginx",
		})
	}

	return result, nil
}

func (cs *containerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	fmt.Printf("Running container %q with name %q\n", r.Image, r.ID)
	return nil
}

func (cs *containerService) Start(ctx context.Context, containerID string) error {
	return errors.New("not implemented")
}

func (cs *containerService) Stop(ctx context.Context, containerName string, timeout *uint32) error {
	return errors.New("not implemented")
}

func (cs *containerService) Exec(ctx context.Context, name string, request containers.ExecRequest) error {
	fmt.Printf("Executing command %q on container %q", request.Command, name)
	return nil
}

func (cs *containerService) Logs(ctx context.Context, containerName string, request containers.LogsRequest) error {
	fmt.Fprintf(request.Writer, "Following logs for container %q", containerName)
	return nil
}

func (cs *containerService) Delete(ctx context.Context, id string, request containers.DeleteRequest) error {
	fmt.Printf("Deleting container %q with force = %t\n", id, request.Force)
	return nil
}

type composeService struct{}

func (cs *composeService) Up(ctx context.Context, opts cli.ProjectOptions) error {
	prj, err := cli.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	fmt.Printf("Up command on project %q", prj.Name)
	return nil
}

func (cs *composeService) Down(ctx context.Context, opts cli.ProjectOptions) error {
	prj, err := cli.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	fmt.Printf("Down command on project %q", prj.Name)
	return nil
}

func (cs *composeService) Ps(ctx context.Context, opts cli.ProjectOptions) ([]ecstypes.ServiceStatus, error) {
	return nil, errdefs.ErrNotImplemented
}

func (cs *composeService) Logs(ctx context.Context, opts cli.ProjectOptions) error {
	return errdefs.ErrNotImplemented
}
