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
	"github.com/docker/compose-cli/utils"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/streams"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

func (s *composeService) attach(ctx context.Context, project *types.Project, listener compose.ContainerEventListener, selectedServices []string) (Containers, error) {
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, true, selectedServices...)
	if err != nil {
		return nil, err
	}

	containers.sorted() // This enforce predictable colors assignment

	var names []string
	for _, c := range containers {
		names = append(names, getContainerNameWithoutProject(c))
	}

	fmt.Printf("Attaching to %s\n", strings.Join(names, ", "))

	for _, container := range containers {
		err := s.attachContainer(ctx, container, listener, project)
		if err != nil {
			return nil, err
		}
	}
	return containers, err
}

func (s *composeService) attachContainer(ctx context.Context, container moby.Container, listener compose.ContainerEventListener, project *types.Project) error {
	serviceName := container.Labels[serviceLabel]
	containerName := getContainerNameWithoutProject(container)
	service, err := project.GetService(serviceName)
	if err != nil {
		return err
	}

	listener(compose.ContainerEvent{
		Type:      compose.ContainerEventAttach,
		Container: containerName,
		Service:   serviceName,
	})

	w := utils.GetWriter(func(line string) {
		listener(compose.ContainerEvent{
			Type:      compose.ContainerEventLog,
			Container: containerName,
			Service:   serviceName,
			Line:      line,
		})
	})
	_, err = s.attachContainerStreams(ctx, container.ID, service.Tty, nil, w)
	return err
}

func (s *composeService) attachContainerStreams(ctx context.Context, container string, tty bool, r io.ReadCloser, w io.Writer) (func(), error) {
	var (
		in      *streams.In
		restore = func() { /* noop */ }
	)
	if r != nil {
		in = streams.NewIn(r)
		restore = in.RestoreTerminal
	}

	stdin, stdout, err := s.getContainerStreams(ctx, container)
	if err != nil {
		return restore, err
	}

	go func() {
		<-ctx.Done()
		if in != nil {
			in.Close() //nolint:errcheck
		}
		stdout.Close() //nolint:errcheck
	}()

	if in != nil && stdin != nil {
		err := in.SetRawTerminal()
		if err != nil {
			return restore, err
		}
		go func() {
			io.Copy(stdin, in) //nolint:errcheck
		}()
	}

	if w != nil {
		go func() {
			if tty {
				io.Copy(w, stdout) // nolint:errcheck
			} else {
				stdcopy.StdCopy(w, w, stdout) // nolint:errcheck
			}
		}()
	}
	return restore, nil
}

func (s *composeService) getContainerStreams(ctx context.Context, container string) (io.WriteCloser, io.ReadCloser, error) {
	var stdout io.ReadCloser
	var stdin io.WriteCloser
	cnx, err := s.apiClient.ContainerAttach(ctx, container, moby.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   false,
	})
	if err == nil {
		stdout = convert.ContainerStdout{HijackedResponse: cnx}
		stdin = convert.ContainerStdin{HijackedResponse: cnx}
		return stdin, stdout, nil
	}

	// Fallback to logs API
	logs, err := s.apiClient.ContainerLogs(ctx, container, moby.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, nil, err
	}
	return stdin, logs, nil
}
