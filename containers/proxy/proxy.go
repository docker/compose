package proxy

import (
	"context"

	"github.com/docker/api/client"
	v1 "github.com/docker/api/containers/v1"
	apicontext "github.com/docker/api/context"
	"github.com/golang/protobuf/ptypes/empty"
)

func NewContainerApi(client *client.Client) v1.ContainersServer {
	return &proxyContainerApi{
		client: client,
	}
}

type proxyContainerApi struct {
	client *client.Client
}

func (p *proxyContainerApi) List(ctx context.Context, _ *v1.ListRequest) (*v1.ListResponse, error) {
	currentContext := apicontext.CurrentContext(ctx)
	if err := p.client.SetContext(ctx, currentContext); err != nil {
		return &v1.ListResponse{}, nil
	}

	c, err := p.client.ContainerService().List(ctx)
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

func (p *proxyContainerApi) Stop(_ context.Context, _ *v1.StopRequest) (*empty.Empty, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Kill(_ context.Context, _ *v1.KillRequest) (*empty.Empty, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Delete(_ context.Context, _ *v1.DeleteRequest) (*empty.Empty, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Update(_ context.Context, _ *v1.UpdateRequest) (*v1.UpdateResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerApi) Exec(_ context.Context, _ *v1.ExecRequest) (*v1.ExecResponse, error) {
	panic("not implemented") // TODO: Implement
}
