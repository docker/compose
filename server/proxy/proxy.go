package proxy

import (
	"context"

	"github.com/docker/api/client"
	"github.com/docker/api/containers"
	v1 "github.com/docker/api/containers/v1"
)

type clientKey struct{}

// WithClient adds the client to the context
func WithClient(ctx context.Context, c *client.Client) (context.Context, error) {
	return context.WithValue(ctx, clientKey{}, c), nil
}

// Client returns the client from the context
func Client(ctx context.Context) *client.Client {
	c, _ := ctx.Value(clientKey{}).(*client.Client)
	return c
}

// NewContainerAPI creates a proxy container server
func NewContainerAPI() v1.ContainersServer {
	return &proxyContainerAPI{}
}

type proxyContainerAPI struct{}

func (p *proxyContainerAPI) List(ctx context.Context, _ *v1.ListRequest) (*v1.ListResponse, error) {
	client := Client(ctx)

	c, err := client.AciService().List(ctx)
	if err != nil {
		return &v1.ListResponse{}, nil
	}

	response := &v1.ListResponse{
		Containers: []*v1.Container{},
	}
	for _, container := range c {
		response.Containers = append(response.Containers, &v1.Container{
			Id:    container.ID,
			Image: container.Image,
		})
	}

	return response, nil
}

func (p *proxyContainerAPI) Create(ctx context.Context, request *v1.CreateRequest) (*v1.CreateResponse, error) {
	client := Client(ctx)

	err := client.AciService().Run(ctx, containers.ContainerConfig{
		ID:    request.Id,
		Image: request.Image,
	})

	return &v1.CreateResponse{}, err
}

func (p *proxyContainerAPI) Start(_ context.Context, _ *v1.StartRequest) (*v1.StartResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Stop(_ context.Context, _ *v1.StopRequest) (*v1.StopResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Kill(_ context.Context, _ *v1.KillRequest) (*v1.KillResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Delete(_ context.Context, _ *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Update(_ context.Context, _ *v1.UpdateRequest) (*v1.UpdateResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Exec(_ context.Context, _ *v1.ExecRequest) (*v1.ExecResponse, error) {
	panic("not implemented") // TODO: Implement
}
