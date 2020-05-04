package example

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/api/backend"
	"github.com/docker/api/containers"
)

type containerService struct{}

func init() {
	backend.Register("example", "example", func(ctx context.Context) (interface{}, error) {
		return &containerService{}, nil
	})
}

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
