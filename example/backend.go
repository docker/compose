package example

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
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
	backend.Register("example", "example", func(ctx context.Context) (backend.Service, error) {
		return &apiService{}, nil
	})
}

type containerService struct{}

func (cs *containerService) List(ctx context.Context) ([]containers.Container, error) {
	return []containers.Container{
		{
			ID:    "id",
			Image: "nginx",
		},
		{
			ID:    "1234",
			Image: "alpine",
		},
	}, nil
}

func (cs *containerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	fmt.Printf("Running container %q with name %q\n", r.Image, r.ID)
	return nil
}

func (cs *containerService) Exec(ctx context.Context, name string, command string, reader io.Reader, writer io.Writer) error {
	fmt.Printf("Executing command %q on container %q", command, name)
	return nil
}

func (cs *containerService) Logs(ctx context.Context, containerName string, request containers.LogsRequest) error {
	fmt.Fprintf(request.Writer, "Following logs for container %q", containerName)
	return nil
}

func (cs *containerService) Delete(ctx context.Context, id string, force bool) error {
	fmt.Printf("Deleting container %q with force = %t\n", id, force)
	return nil
}

type composeService struct{}

func (cs *composeService) Up(ctx context.Context, opts compose.ProjectOptions) error {
	prj, err := compose.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	fmt.Printf("Up command on project %q", prj.Name)
	return nil
}

func (cs *composeService) Down(ctx context.Context, opts compose.ProjectOptions) error {
	prj, err := compose.ProjectFromOptions(&opts)
	if err != nil {
		return err
	}
	fmt.Printf("Down command on project %q", prj.Name)
	return nil
}
