package proxy

import (
	"context"
	"errors"

	"github.com/docker/api/containers"
	containersv1 "github.com/docker/api/protos/containers/v1"
)

func portsToGrpc(ports []containers.Port) []*containersv1.Port {
	var result []*containersv1.Port
	for _, port := range ports {
		result = append(result, &containersv1.Port{
			ContainerPort: port.ContainerPort,
			HostPort:      port.HostPort,
			HostIp:        port.HostIP,
			Protocol:      port.Protocol,
		})
	}

	return result
}

func (p *proxy) List(ctx context.Context, request *containersv1.ListRequest) (*containersv1.ListResponse, error) {
	client := Client(ctx)

	c, err := client.ContainerService().List(ctx, request.GetAll())
	if err != nil {
		return &containersv1.ListResponse{}, err
	}

	response := &containersv1.ListResponse{
		Containers: []*containersv1.Container{},
	}
	for _, container := range c {
		response.Containers = append(response.Containers, &containersv1.Container{
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

func (p *proxy) Stop(ctx context.Context, request *containersv1.StopRequest) (*containersv1.StopResponse, error) {
	c := Client(ctx)
	timeoutValue := request.GetTimeout()
	return &containersv1.StopResponse{}, c.ContainerService().Stop(ctx, request.Id, &timeoutValue)
}

func (p *proxy) Run(ctx context.Context, request *containersv1.RunRequest) (*containersv1.RunResponse, error) {
	ports := []containers.Port{}
	for _, p := range request.GetPorts() {
		ports = append(ports, containers.Port{
			ContainerPort: p.ContainerPort,
			HostIP:        p.HostIp,
			HostPort:      p.HostPort,
			Protocol:      p.Protocol,
		})
	}

	err := Client(ctx).ContainerService().Run(ctx, containers.ContainerConfig{
		ID:      request.GetId(),
		Image:   request.GetImage(),
		Labels:  request.GetLabels(),
		Ports:   ports,
		Volumes: request.GetVolumes(),
	})

	return &containersv1.RunResponse{}, err
}

func (p *proxy) Delete(ctx context.Context, request *containersv1.DeleteRequest) (*containersv1.DeleteResponse, error) {
	err := Client(ctx).ContainerService().Delete(ctx, request.Id, request.Force)
	if err != nil {
		return &containersv1.DeleteResponse{}, err
	}

	return &containersv1.DeleteResponse{}, nil
}

func (p *proxy) Exec(ctx context.Context, request *containersv1.ExecRequest) (*containersv1.ExecResponse, error) {
	p.mu.Lock()
	stream, ok := p.streams[request.StreamId]
	p.mu.Unlock()
	if !ok {
		return &containersv1.ExecResponse{}, errors.New("unknown stream id")
	}

	err := Client(ctx).ContainerService().Exec(ctx, request.GetId(), request.GetCommand(), &reader{stream}, &writer{stream})

	return &containersv1.ExecResponse{}, err
}

func (p *proxy) Logs(request *containersv1.LogsRequest, stream containersv1.Containers_LogsServer) error {
	ctx := stream.Context()
	c := Client(ctx)

	return c.ContainerService().Logs(ctx, request.GetContainerId(), containers.LogsRequest{
		Follow: request.Follow,
		Writer: &logStream{stream},
	})
}
