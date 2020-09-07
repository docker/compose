// +build local

/*
   Copyright 2020 Docker, Inc.

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

package local

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stringid"

	"github.com/docker/go-connections/nat"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/api/secrets"
	"github.com/docker/compose-cli/backend"
	"github.com/docker/compose-cli/context/cloud"
	"github.com/docker/compose-cli/errdefs"
)

type local struct {
	apiClient *client.Client
}

func init() {
	backend.Register("local", "local", service, cloud.NotImplementedCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &local{
		apiClient,
	}, nil
}

func (ms *local) ContainerService() containers.Service {
	return ms
}

func (ms *local) ComposeService() compose.Service {
	return nil
}

func (ms *local) SecretsService() secrets.Service {
	return nil
}

func (ms *local) Inspect(ctx context.Context, id string) (containers.Container, error) {
	c, err := ms.apiClient.ContainerInspect(ctx, id)
	if err != nil {
		return containers.Container{}, err
	}

	status := ""
	if c.State != nil {
		status = c.State.Status
	}

	command := ""
	if c.Config != nil &&
		c.Config.Cmd != nil {
		command = strings.Join(c.Config.Cmd, " ")
	}

	return containers.Container{
		ID:       stringid.TruncateID(c.ID),
		Status:   status,
		Image:    c.Image,
		Command:  command,
		Platform: c.Platform,
	}, nil
}

func (ms *local) List(ctx context.Context, all bool) ([]containers.Container, error) {
	css, err := ms.apiClient.ContainerList(ctx, types.ContainerListOptions{
		All: all,
	})

	if err != nil {
		return []containers.Container{}, err
	}

	var result []containers.Container
	for _, container := range css {
		result = append(result, containers.Container{
			ID:    stringid.TruncateID(container.ID),
			Image: container.Image,
			// TODO: `Status` is a human readable string ("Up 24 minutes"),
			// we need to return the `State` instead but first we need to
			// define an enum on the proto side with all the possible container
			// statuses. We also need to add a `Created` property on the gRPC side.
			Status:  container.Status,
			Command: container.Command,
			Ports:   toPorts(container.Ports),
		})
	}

	return result, nil
}

func (ms *local) Run(ctx context.Context, r containers.ContainerConfig) error {
	exposedPorts, hostBindings, err := fromPorts(r.Ports)
	if err != nil {
		return err
	}

	containerConfig := &container.Config{
		Image:        r.Image,
		Labels:       r.Labels,
		ExposedPorts: exposedPorts,
	}
	hostConfig := &container.HostConfig{
		PortBindings: hostBindings,
	}

	created, err := ms.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, r.ID)

	if err != nil {
		if client.IsErrNotFound(err) {
			io, err := ms.apiClient.ImagePull(ctx, r.Image, types.ImagePullOptions{})
			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(io)

			// Read the whole body, otherwise the pulling stops
			for scanner.Scan() {
			}

			if err = scanner.Err(); err != nil {
				return err
			}
			if err = io.Close(); err != nil {
				return err
			}
			created, err = ms.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, r.ID)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return ms.apiClient.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
}

func (ms *local) Start(ctx context.Context, containerID string) error {
	return ms.apiClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

func (ms *local) Stop(ctx context.Context, containerID string, timeout *uint32) error {
	var t *time.Duration
	if timeout != nil {
		timeoutValue := time.Duration(*timeout) * time.Second
		t = &timeoutValue
	}
	return ms.apiClient.ContainerStop(ctx, containerID, t)
}

func (ms *local) Kill(ctx context.Context, containerID string, signal string) error {
	return ms.apiClient.ContainerKill(ctx, containerID, signal)
}

func (ms *local) Exec(ctx context.Context, name string, request containers.ExecRequest) error {
	cec, err := ms.apiClient.ContainerExecCreate(ctx, name, types.ExecConfig{
		Cmd:          []string{request.Command},
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return err
	}
	resp, err := ms.apiClient.ContainerExecAttach(ctx, cec.ID, types.ExecStartCheck{
		Tty: true,
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	readChannel := make(chan error, 10)
	writeChannel := make(chan error, 10)

	go func() {
		_, err := io.Copy(request.Stdout, resp.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(resp.Conn, request.Stdin)
		writeChannel <- err
	}()

	for {
		select {
		case err := <-readChannel:
			return err
		case err := <-writeChannel:
			return err
		}
	}
}

func (ms *local) Logs(ctx context.Context, containerName string, request containers.LogsRequest) error {
	c, err := ms.apiClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return err
	}

	r, err := ms.apiClient.ContainerLogs(ctx, containerName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     request.Follow,
	})

	if err != nil {
		return err
	}

	// nolint errcheck
	defer r.Close()

	if c.Config.Tty {
		_, err = io.Copy(request.Writer, r)
	} else {
		_, err = stdcopy.StdCopy(request.Writer, request.Writer, r)
	}

	return err
}

func (ms *local) Delete(ctx context.Context, containerID string, request containers.DeleteRequest) error {
	err := ms.apiClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: request.Force,
	})
	if client.IsErrNotFound(err) {
		return errors.Wrapf(errdefs.ErrNotFound, "container %q", containerID)
	}
	return err
}

func toPorts(ports []types.Port) []containers.Port {
	result := []containers.Port{}
	for _, port := range ports {
		result = append(result, containers.Port{
			ContainerPort: uint32(port.PrivatePort),
			HostPort:      uint32(port.PublicPort),
			HostIP:        port.IP,
			Protocol:      port.Type,
		})
	}

	return result
}

func fromPorts(ports []containers.Port) (map[nat.Port]struct{}, map[nat.Port][]nat.PortBinding, error) {
	var (
		exposedPorts = make(map[nat.Port]struct{}, len(ports))
		bindings     = make(map[nat.Port][]nat.PortBinding)
	)

	for _, port := range ports {
		p, err := nat.NewPort(port.Protocol, strconv.Itoa(int(port.ContainerPort)))
		if err != nil {
			return nil, nil, err
		}

		if _, exists := exposedPorts[p]; !exists {
			exposedPorts[p] = struct{}{}
		}

		portBinding := nat.PortBinding{
			HostIP:   port.HostIP,
			HostPort: strconv.Itoa(int(port.HostPort)),
		}
		bslice, exists := bindings[p]
		if !exists {
			bslice = []nat.PortBinding{}
		}
		bindings[p] = append(bslice, portBinding)
	}

	return exposedPorts, bindings, nil
}
