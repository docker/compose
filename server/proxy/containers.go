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

package proxy

import (
	"context"
	"errors"

	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/formatter"
	containersv1 "github.com/docker/compose-cli/protos/containers/v1"
	"github.com/docker/compose-cli/server/proxy/streams"
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
		response.Containers = append(response.Containers, toGrpcContainer(container))
	}

	return response, nil
}

func (p *proxy) Start(ctx context.Context, request *containersv1.StartRequest) (*containersv1.StartResponse, error) {
	return &containersv1.StartResponse{}, Client(ctx).ContainerService().Start(ctx, request.Id)
}

func (p *proxy) Stop(ctx context.Context, request *containersv1.StopRequest) (*containersv1.StopResponse, error) {
	timeoutValue := request.GetTimeout()
	return &containersv1.StopResponse{}, Client(ctx).ContainerService().Stop(ctx, request.Id, &timeoutValue)
}

func (p *proxy) Kill(ctx context.Context, request *containersv1.KillRequest) (*containersv1.KillResponse, error) {
	signal := request.GetSignal()
	return &containersv1.KillResponse{}, Client(ctx).ContainerService().Kill(ctx, request.Id, signal)
}

func (p *proxy) Run(ctx context.Context, request *containersv1.RunRequest) (*containersv1.RunResponse, error) {
	return &containersv1.RunResponse{}, Client(ctx).ContainerService().Run(ctx, grpcContainerToContainerConfig(request))
}

func (p *proxy) Inspect(ctx context.Context, request *containersv1.InspectRequest) (*containersv1.InspectResponse, error) {
	c, err := Client(ctx).ContainerService().Inspect(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	response := &containersv1.InspectResponse{
		Container: toGrpcContainer(c),
	}
	return response, err
}

func (p *proxy) Delete(ctx context.Context, request *containersv1.DeleteRequest) (*containersv1.DeleteResponse, error) {
	return &containersv1.DeleteResponse{}, Client(ctx).ContainerService().Delete(ctx, request.Id, containers.DeleteRequest{
		Force: request.Force,
	})
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

	return &containersv1.ExecResponse{}, Client(ctx).ContainerService().Exec(ctx, request.GetId(), containers.ExecRequest{
		Stdin:   io,
		Stdout:  io,
		Command: request.GetCommand(),
		Tty:     request.GetTty(),
	})
}

func (p *proxy) Logs(request *containersv1.LogsRequest, stream containersv1.Containers_LogsServer) error {
	return Client(stream.Context()).ContainerService().Logs(stream.Context(), request.GetContainerId(), containers.LogsRequest{
		Follow: request.Follow,
		Writer: &streams.Log{
			Stream: stream,
		},
	})
}

func toGrpcContainer(c containers.Container) *containersv1.Container {
	return &containersv1.Container{
		Id:                     c.ID,
		Image:                  c.Image,
		Status:                 c.Status,
		Command:                c.Command,
		CpuTime:                c.CPUTime,
		MemoryUsage:            c.MemoryUsage,
		MemoryLimit:            c.MemoryLimit,
		Platform:               c.Platform,
		PidsCurrent:            c.PidsCurrent,
		PidsLimit:              c.PidsLimit,
		Labels:                 c.Labels,
		Ports:                  portsToGrpc(c.Ports),
		CpuLimit:               uint64(c.CPULimit),
		RestartPolicyCondition: c.RestartPolicyCondition,
	}
}

func grpcContainerToContainerConfig(request *containersv1.RunRequest) containers.ContainerConfig {
	var ports []containers.Port
	for _, p := range request.GetPorts() {
		ports = append(ports, containers.Port{
			ContainerPort: p.ContainerPort,
			HostIP:        p.HostIp,
			HostPort:      p.HostPort,
			Protocol:      p.Protocol,
		})
	}

	return containers.ContainerConfig{
		ID:                     request.GetId(),
		Image:                  request.GetImage(),
		Command:                request.GetCommand(),
		Ports:                  ports,
		Labels:                 request.GetLabels(),
		Volumes:                request.GetVolumes(),
		MemLimit:               formatter.MemBytes(request.GetMemoryLimit()),
		CPULimit:               float64(request.GetCpuLimit()),
		RestartPolicyCondition: request.RestartPolicyCondition,
	}
}
