package proxy

import (
	"context"

	"github.com/docker/api/client"
	v1 "github.com/docker/api/containers/v1"
)

type clientKey struct{}

func WithClient(ctx context.Context, c *client.Client) (context.Context, error) {
	return context.WithValue(ctx, clientKey{}, c), nil
}

func Client(ctx context.Context) *client.Client {
	c, _ := ctx.Value(clientKey{}).(*client.Client)
	return c
}

func NewContainerApi() v1.ContainersServer {
	return &proxyContainerApi{}
}

type proxyContainerApi struct{}

func (p *proxyContainerApi) List(ctx context.Context, _ *v1.ListRequest) (*v1.ListResponse, error) {
	client := Client(ctx)

	c, err := client.ContainerService().List(ctx)
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

func (p *proxyContainerApi) Create(_ context.Context, _ *v1.CreateRequest) (*v1.CreateResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Start(_ context.Context, _ *v1.StartRequest) (*v1.StartResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Stop(_ context.Context, _ *v1.StopRequest) (*v1.StopResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Kill(_ context.Context, _ *v1.KillRequest) (*v1.KillResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Delete(_ context.Context, _ *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Update(_ context.Context, _ *v1.UpdateRequest) (*v1.UpdateResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Exec(_ context.Context, _ *v1.ExecRequest) (*v1.ExecResponse, error) {
	panic("not implemented") // TODO: Implement
}
