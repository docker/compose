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

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/streams"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/term"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

func (s *composeService) attach(ctx context.Context, project *types.Project, listener api.ContainerEventListener, selectedServices []string) (Containers, error) {
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

func (s *composeService) attachContainer(ctx context.Context, container moby.Container, listener api.ContainerEventListener, project *types.Project) error {
	serviceName := container.Labels[api.ServiceLabel]
	containerName := getContainerNameWithoutProject(container)
	service, err := project.GetService(serviceName)
	if err != nil {
		return err
	}

	listener(api.ContainerEvent{
		Type:      api.ContainerEventAttach,
		Container: containerName,
		Service:   serviceName,
	})

	w := utils.GetWriter(func(line string) {
		listener(api.ContainerEvent{
			Type:      api.ContainerEventLog,
			Container: containerName,
			Service:   serviceName,
			Line:      line,
		})
	})
	_, _, err = s.attachContainerStreams(ctx, container.ID, service.Tty, nil, w, w)
	return err
}

func (s *composeService) attachContainerStreams(ctx context.Context, container string, tty bool, stdin io.ReadCloser, stdout, stderr io.Writer) (func(), chan bool, error) {
	detached := make(chan bool)
	var (
		restore = func() { /* noop */ }
	)
	if stdin != nil {
		in := streams.NewIn(stdin)
		if in.IsTerminal() {
			state, err := term.SetRawTerminal(in.FD())
			if err != nil {
				return restore, detached, err
			}
			restore = func() {
				term.RestoreTerminal(in.FD(), state) //nolint:errcheck
			}
		}
	}

	streamIn, streamOut, err := s.getContainerStreams(ctx, container)
	if err != nil {
		return restore, detached, err
	}

	go func() {
		<-ctx.Done()
		if stdin != nil {
			stdin.Close() //nolint:errcheck
		}
		streamOut.Close() //nolint:errcheck
	}()

	if streamIn != nil && stdin != nil {
		go func() {
			_, err := io.Copy(streamIn, stdin)
			if _, ok := err.(term.EscapeError); ok {
				close(detached)
			}
		}()
	}

	if stdout != nil {
		go func() {
			if tty {
				io.Copy(stdout, streamOut) // nolint:errcheck
			} else {
				stdcopy.StdCopy(stdout, stderr, streamOut) // nolint:errcheck
			}
		}()
	}
	return restore, detached, nil
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
		stdout = ContainerStdout{HijackedResponse: cnx}
		stdin = ContainerStdin{HijackedResponse: cnx}
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
