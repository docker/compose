package moby

import (
	"context"
	"io"

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
	backend.Register("moby", "moby", func(ctx context.Context) (backend.Service, error) {
		return New()
	})
}

// New returns a moby backend implementation
func New() (backend.Service, error) {
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

func (ms *mobyService) CloudService() cloud.Service {
	return nil
}

func (ms *mobyService) List(ctx context.Context) ([]containers.Container, error) {
	css, err := ms.apiClient.ContainerList(ctx, types.ContainerListOptions{
		All: false,
	})
	if err != nil {
		return []containers.Container{}, err
	}

	var result []containers.Container
	for _, container := range css {
		result = append(result, containers.Container{
			ID:      container.ID,
			Image:   container.Image,
			Status:  container.Status,
			Command: container.Command,
			Ports:   getPorts(container.Ports),
		})
	}

	return result, nil
}

func (ms *mobyService) Run(ctx context.Context, r containers.ContainerConfig) error {
	create, err := ms.apiClient.ContainerCreate(ctx, &container.Config{
		Image:  r.Image,
		Labels: r.Labels,
	}, nil, nil, r.ID)
	if err != nil {
		return err
	}

	return ms.apiClient.ContainerStart(ctx, create.ID, types.ContainerStartOptions{})
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

func getPorts(ports []types.Port) []containers.Port {
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
