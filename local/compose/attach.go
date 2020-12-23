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

package compose

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/compose-cli/api/compose"
	convert "github.com/docker/compose-cli/local/moby"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) attach(ctx context.Context, project *types.Project, consumer compose.LogConsumer) (*errgroup.Group, error) {
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(project.Name),
		),
		All: true,
	})
	if err != nil {
		return nil, err
	}

	var names []string
	for _, c := range containers {
		names = append(names, getContainerName(c))
	}
	fmt.Printf("Attaching to %s\n", strings.Join(names, ", "))

	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		container := c
		eg.Go(func() error {
			return s.attachContainer(ctx, container, consumer, project)
		})
	}
	return eg, nil
}

func (s *composeService) attachContainer(ctx context.Context, container moby.Container, consumer compose.LogConsumer, project *types.Project) error {
	serviceName := container.Labels[serviceLabel]
	w := getWriter(serviceName, getContainerName(container), consumer)

	service, err := project.GetService(serviceName)
	if err != nil {
		return err
	}

	return s.attachContainerStreams(ctx, container, service.Tty, nil, w)
}

func (s *composeService) attachContainerStreams(ctx context.Context, container moby.Container, tty bool, r io.Reader, w io.Writer) error {
	stdin, stdout, err := s.getContainerStreams(ctx, container)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		stdout.Close() //nolint:errcheck
		if stdin != nil {
			stdin.Close() //nolint:errcheck
		}
	}()

	if r != nil && stdin != nil {
		go func() {
			io.Copy(stdin, r) //nolint:errcheck
		}()
	}

	if w != nil {
		if tty {
			_, err = io.Copy(w, stdout)
		} else {
			_, err = stdcopy.StdCopy(w, w, stdout)
		}
	}
	return err
}

func (s *composeService) getContainerStreams(ctx context.Context, container moby.Container) (io.WriteCloser, io.ReadCloser, error) {
	var stdout io.ReadCloser
	var stdin io.WriteCloser
	if container.State == convert.ContainerRunning {
		logs, err := s.apiClient.ContainerLogs(ctx, container.ID, moby.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			return nil, nil, err
		}
		stdout = logs
	} else {
		cnx, err := s.apiClient.ContainerAttach(ctx, container.ID, moby.ContainerAttachOptions{
			Stream: true,
			Stdin:  true,
			Stdout: true,
			Stderr: true,
			Logs:   true,
		})
		if err != nil {
			return nil, nil, err
		}
		stdout = convert.ContainerStdout{HijackedResponse: cnx}
		stdin = convert.ContainerStdin{HijackedResponse: cnx}
	}
	return stdin, stdout, nil
}
