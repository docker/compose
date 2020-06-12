package proxy

import (
	"context"
	"errors"

	"github.com/docker/api/containers"
	containersv1 "github.com/docker/api/protos/containers/v1"
	"github.com/docker/api/server/proxy/streams"
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
	containerList, err := Client(ctx).ContainerService().List(ctx, request.GetAll())
	if err != nil {
		return &containersv1.ListResponse{}, err
	}

	response := &containersv1.ListResponse{
		Containers: []*containersv1.Container{},
	}
	for _, container := range containerList {
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
	timeoutValue := request.GetTimeout()
	return &containersv1.StopResponse{}, Client(ctx).ContainerService().Stop(ctx, request.Id, &timeoutValue)
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

	return &containersv1.RunResponse{}, Client(ctx).ContainerService().Run(ctx, containers.ContainerConfig{
		ID:      request.GetId(),
		Image:   request.GetImage(),
		Labels:  request.GetLabels(),
		Ports:   ports,
		Volumes: request.GetVolumes(),
	})
}

func (p *proxy) Inspect(ctx context.Context, request *containersv1.InspectRequest) (*containersv1.InspectResponse, error) {
	c, err := Client(ctx).ContainerService().Inspect(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	response := &containersv1.InspectResponse{
		Container: &containersv1.Container{
			Id:          c.ID,
			Image:       c.Image,
			Status:      c.Status,
			Command:     c.Command,
			CpuTime:     c.CPUTime,
			MemoryUsage: c.MemoryUsage,
			MemoryLimit: c.MemoryLimit,
			PidsCurrent: c.PidsCurrent,
			PidsLimit:   c.PidsLimit,
			Labels:      c.Labels,
			Ports:       portsToGrpc(c.Ports),
		},
	}
	return response, err
}

func (p *proxy) Delete(ctx context.Context, request *containersv1.DeleteRequest) (*containersv1.DeleteResponse, error) {
	return &containersv1.DeleteResponse{}, Client(ctx).ContainerService().Delete(ctx, request.Id, request.Force)
}

func (p *proxy) Exec(ctx context.Context, request *containersv1.ExecRequest) (*containersv1.ExecResponse, error) {
	p.mu.Lock()
	stream, ok := p.streams[request.StreamId]
	p.mu.Unlock()
	if !ok {
		return &containersv1.ExecResponse{}, errors.New("unknown stream id")
	}

	io := &streams.IO{
		Stream: stream,
	}

	return &containersv1.ExecResponse{}, Client(ctx).ContainerService().Exec(ctx, request.GetId(), request.GetCommand(), io, io)
}

func (p *proxy) Logs(request *containersv1.LogsRequest, stream containersv1.Containers_LogsServer) error {
	return Client(stream.Context()).ContainerService().Logs(stream.Context(), request.GetContainerId(), containers.LogsRequest{
		Follow: request.Follow,
		Writer: &streams.Log{
			Stream: stream,
		},
	})
}
