// +build example

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

package example

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/resources"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/api/volumes"
	"github.com/docker/compose-cli/backend"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/errdefs"
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

func (a *apiService) SecretsService() secrets.Service {
	return nil
}

func (a *apiService) VolumeService() volumes.Service {
	return nil
}

func (a *apiService) ResourceService() resources.Service {
	return nil
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
		ID:       "id",
		Image:    "nginx",
		Platform: "Linux",
		HostConfig: &containers.HostConfig{
			RestartPolicy: "none",
		},
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

func (cs *containerService) Kill(ctx context.Context, containerName string, signal string) error {
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

func (cs *composeService) Build(ctx context.Context, project *types.Project) error {
	fmt.Printf("Build command on project %q", project.Name)
	return nil
}

func (cs *composeService) Push(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (cs *composeService) Pull(ctx context.Context, project *types.Project) error {
	return errdefs.ErrNotImplemented
}

func (cs *composeService) Up(ctx context.Context, project *types.Project, detach bool) error {
	fmt.Printf("Up command on project %q", project.Name)
	return nil
}

func (cs *composeService) Down(ctx context.Context, project string) error {
	fmt.Printf("Down command on project %q", project)
	return nil
}

func (cs *composeService) Ps(ctx context.Context, project string) ([]compose.ServiceStatus, error) {
	return nil, errdefs.ErrNotImplemented
}
func (cs *composeService) List(ctx context.Context, project string) ([]compose.Stack, error) {
	return nil, errdefs.ErrNotImplemented
}
func (cs *composeService) Logs(ctx context.Context, project string, w io.Writer) error {
	return errdefs.ErrNotImplemented
}

func (cs *composeService) Convert(ctx context.Context, project *types.Project, format string) ([]byte, error) {
	return nil, errdefs.ErrNotImplemented
}
