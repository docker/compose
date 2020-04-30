package example

import (
	"context"

	"github.com/docker/api/backend"
	"github.com/docker/api/containers"
)

type containerService struct{}

func init() {
	backend.Register("example", "example", func(ctx context.Context) (interface{}, error) {
		return New(), nil
	})
}

func New() containers.ContainerService {
	return &containerService{}
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
