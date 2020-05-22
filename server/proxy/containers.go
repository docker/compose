package proxy

import (
	"context"

	"github.com/docker/api/containers"
	v1 "github.com/docker/api/containers/v1"
)

// NewContainerAPI creates a proxy container server
func NewContainerAPI() v1.ContainersServer {
	return &proxyContainerAPI{}
}

type proxyContainerAPI struct {
}

func portsToGrpc(ports []containers.Port) []*v1.Port {
	var result []*v1.Port
	for _, port := range ports {
		result = append(result, &v1.Port{
			ContainerPort: port.ContainerPort,
			HostPort:      port.HostPort,
			HostIp:        port.HostIP,
			Protocol:      port.Protocol,
		})
	}

	return result
}

func (p *proxyContainerAPI) List(ctx context.Context, request *v1.ListRequest) (*v1.ListResponse, error) {
	client := Client(ctx)

	c, err := client.ContainerService().List(ctx, request.GetAll())
	if err != nil {
		return &v1.ListResponse{}, nil
	}

	response := &v1.ListResponse{
		Containers: []*v1.Container{},
	}
	for _, container := range c {
		response.Containers = append(response.Containers, &v1.Container{
			Id:          container.ID,
			Image:       container.Image,
			Command:     container.Command,
			Status:      container.Status,
			CpuTime:     container.CPUTime,
			Labels:      container.Labels,
			MemoryLimit: container.MemoryLimit,
			MemoryUsage: container.MemoryUsage,
			PidsCurrent: container.PidsCurrent,
			PidsLimit:   container.PidsLimit,
			Ports:       portsToGrpc(container.Ports),
		})
	}

	return response, nil
}

func (p *proxyContainerAPI) Create(ctx context.Context, request *v1.CreateRequest) (*v1.CreateResponse, error) {
	client := Client(ctx)

	err := client.ContainerService().Run(ctx, containers.ContainerConfig{
		ID:    request.Id,
		Image: request.Image,
	})

	return &v1.CreateResponse{}, err
}

func (p *proxyContainerAPI) Start(_ context.Context, request *v1.StartRequest) (*v1.StartResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Stop(ctx context.Context, request *v1.StopRequest) (*v1.StopResponse, error) {
	c := Client(ctx)
	timeoutValue := request.GetTimeout()
	return &v1.StopResponse{}, c.ContainerService().Stop(ctx, request.Id, &timeoutValue)
}

func (p *proxyContainerAPI) Kill(ctx context.Context, request *v1.KillRequest) (*v1.KillResponse, error) {
	c := Client(ctx)
	return &v1.KillResponse{}, c.ContainerService().Delete(ctx, request.Id, false)
}

func (p *proxyContainerAPI) Delete(ctx context.Context, request *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	err := Client(ctx).ContainerService().Delete(ctx, request.Id, request.Force)
	if err != nil {
		return &v1.DeleteResponse{}, err
	}

	return &v1.DeleteResponse{}, nil
}

func (p *proxyContainerAPI) Update(_ context.Context, _ *v1.UpdateRequest) (*v1.UpdateResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Exec(_ context.Context, _ *v1.ExecRequest) (*v1.ExecResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (p *proxyContainerAPI) Logs(request *v1.LogsRequest, stream v1.Containers_LogsServer) error {
	ctx := stream.Context()
	c := Client(ctx)

	return c.ContainerService().Logs(ctx, request.GetContainerId(), containers.LogsRequest{
		Follow: request.Follow,
		Writer: &streamWriter{stream},
	})
}
