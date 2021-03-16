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
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

func (s *composeService) attach(ctx context.Context, project *types.Project, listener compose.ContainerEventListener, selectedServices []string) (Containers, error) {
	containers, err := s.getContainers(ctx, project, oneOffExclude, selectedServices)
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

	// Watch events to capture container restart and re-attach
	go func() {
		crashed := map[string]struct{}{}
		s.Events(ctx, project.Name, compose.EventsOptions{ // nolint: errcheck
			Services: selectedServices,
			Consumer: func(event compose.Event) error {
				if event.Status == "die" {
					crashed[event.Container] = struct{}{}
					return nil
				}
				if _, ok := crashed[event.Container]; ok {
					inspect, err := s.apiClient.ContainerInspect(ctx, event.Container)
					if err != nil {
						return err
					}

					container := moby.Container{
						ID:    event.Container,
						Names: []string{inspect.Name},
						State: convert.ContainerRunning,
						Labels: map[string]string{
							projectLabel: project.Name,
							serviceLabel: event.Service,
						},
					}

					// Just ignore errors when reattaching to already crashed containers
					s.attachContainer(ctx, container, listener, project) // nolint: errcheck
					delete(crashed, event.Container)

					s.waitContainer(ctx, container, listener)
				}
				return nil
			},
		})
	}()

	return containers, err
}

func (s *composeService) attachContainer(ctx context.Context, container moby.Container, listener compose.ContainerEventListener, project *types.Project) error {
	serviceName := container.Labels[serviceLabel]
	w := utils.GetWriter(getContainerNameWithoutProject(container), serviceName, listener)

	service, err := project.GetService(serviceName)
	if err != nil {
		return err
	}

	listener(compose.ContainerEvent{
		Type:      compose.ContainerEventAttach,
		Container: getContainerNameWithoutProject(container),
		Service:   container.Labels[serviceLabel],
	})

	return s.attachContainerStreams(ctx, container.ID, service.Tty, nil, w)
}

func (s *composeService) attachContainerStreams(ctx context.Context, container string, tty bool, r io.Reader, w io.Writer) error {
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
		go func() {
			if tty {
				io.Copy(w, stdout) // nolint:errcheck
			} else {
				stdcopy.StdCopy(w, w, stdout) // nolint:errcheck
			}
		}()
	}
	return nil
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
