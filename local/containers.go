// +build local

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

package local

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/stringid"
	"github.com/pkg/errors"

	"github.com/docker/compose-cli/api/containers"
	"github.com/docker/compose-cli/errdefs"
)

type containerService struct {
	apiClient *client.Client
}

func (cs *containerService) Inspect(ctx context.Context, id string) (containers.Container, error) {
	c, err := cs.apiClient.ContainerInspect(ctx, id)
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

	rc := toRuntimeConfig(&c)
	hc := toHostConfig(&c)

	return containers.Container{
		ID:         stringid.TruncateID(c.ID),
		Status:     status,
		Image:      c.Image,
		Command:    command,
		Platform:   c.Platform,
		Config:     rc,
		HostConfig: hc,
	}, nil
}

func (cs *containerService) List(ctx context.Context, all bool) ([]containers.Container, error) {
	css, err := cs.apiClient.ContainerList(ctx, types.ContainerListOptions{
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

func (cs *containerService) Run(ctx context.Context, r containers.ContainerConfig) error {
	exposedPorts, hostBindings, err := fromPorts(r.Ports)
	if err != nil {
		return err
	}

	containerConfig := &container.Config{
		Image:        r.Image,
		Labels:       r.Labels,
		Env:          r.Environment,
		ExposedPorts: exposedPorts,
	}
	hostConfig := &container.HostConfig{
		PortBindings:  hostBindings,
		AutoRemove:    r.AutoRemove,
		RestartPolicy: toRestartPolicy(r.RestartPolicyCondition),
		Resources: container.Resources{
			NanoCPUs: int64(r.CPULimit * 1e9),
			Memory:   int64(r.MemLimit),
		},
	}

	created, err := cs.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, r.ID)

	if err != nil {
		if client.IsErrNotFound(err) {
			io, err := cs.apiClient.ImagePull(ctx, r.Image, types.ImagePullOptions{})
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
			created, err = cs.apiClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, r.ID)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return cs.apiClient.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
}

func (cs *containerService) Start(ctx context.Context, containerID string) error {
	return cs.apiClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

func (cs *containerService) Stop(ctx context.Context, containerID string, timeout *uint32) error {
	var t *time.Duration
	if timeout != nil {
		timeoutValue := time.Duration(*timeout) * time.Second
		t = &timeoutValue
	}
	return cs.apiClient.ContainerStop(ctx, containerID, t)
}

func (cs *containerService) Kill(ctx context.Context, containerID string, signal string) error {
	return cs.apiClient.ContainerKill(ctx, containerID, signal)
}

func (cs *containerService) Exec(ctx context.Context, name string, request containers.ExecRequest) error {
	cec, err := cs.apiClient.ContainerExecCreate(ctx, name, types.ExecConfig{
		Cmd:          []string{request.Command},
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return err
	}
	resp, err := cs.apiClient.ContainerExecAttach(ctx, cec.ID, types.ExecStartCheck{
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

func (cs *containerService) Logs(ctx context.Context, containerName string, request containers.LogsRequest) error {
	c, err := cs.apiClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return err
	}

	r, err := cs.apiClient.ContainerLogs(ctx, containerName, types.ContainerLogsOptions{
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

func (cs *containerService) Delete(ctx context.Context, containerID string, request containers.DeleteRequest) error {
	err := cs.apiClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: request.Force,
	})
	if client.IsErrNotFound(err) {
		return errors.Wrapf(errdefs.ErrNotFound, "container %q", containerID)
	}
	return err
}
