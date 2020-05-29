package moby

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/docker/api/context/cloud"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/docker/api/backend"
	"github.com/docker/api/compose"
	"github.com/docker/api/containers"
	"github.com/docker/api/errdefs"
)

type mobyService struct {
	apiClient *client.Client
}

func init() {
	backend.Register("moby", "moby", service, cloud.NotImplementedCloudService)
}

func service(ctx context.Context) (backend.Service, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &mobyService{
		apiClient,
	}, nil
}

func (ms *mobyService) ContainerService() containers.Service {
	return ms
}

func (ms *mobyService) ComposeService() compose.Service {
	return nil
}

func (ms *mobyService) List(ctx context.Context, all bool) ([]containers.Container, error) {
	css, err := ms.apiClient.ContainerList(ctx, types.ContainerListOptions{
		All: all,
	})

	if err != nil {
		return []containers.Container{}, err
	}

	var result []containers.Container
	for _, container := range css {
		result = append(result, containers.Container{
			ID:    container.ID,
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

func (ms *mobyService) Run(ctx context.Context, r containers.ContainerConfig) error {
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

func (ms *mobyService) Stop(ctx context.Context, containerID string, timeout *uint32) error {
	var t *time.Duration
	if timeout != nil {
		timeoutValue := time.Duration(*timeout) * time.Second
		t = &timeoutValue
	}
	return ms.apiClient.ContainerStop(ctx, containerID, t)
}

func (ms *mobyService) Exec(ctx context.Context, name string, command string, reader io.Reader, writer io.Writer) error {
	cec, err := ms.apiClient.ContainerExecCreate(ctx, name, types.ExecConfig{
		Cmd:          []string{command},
		Tty:          true,
		AttachStderr: true,
		AttachStdin:  true,
		AttachStdout: true,
	})
	if err != nil {
		return err
	}
	resp, err := ms.apiClient.ContainerExecAttach(ctx, cec.ID, types.ExecStartCheck{})
	if err != nil {
		return err
	}
	defer resp.Close()
	readChannel := make(chan error, 10)
	writeChannel := make(chan error, 10)

	go func() {
		_, err := io.Copy(writer, resp.Reader)
		readChannel <- err
	}()

	go func() {
		_, err := io.Copy(resp.Conn, reader)
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

func (ms *mobyService) Logs(ctx context.Context, containerName string, request containers.LogsRequest) error {
	r, err := ms.apiClient.ContainerLogs(ctx, containerName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     request.Follow,
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(request.Writer, r)
	return err
}

func (ms *mobyService) Delete(ctx context.Context, containerID string, force bool) error {
	err := ms.apiClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: force,
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
