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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

func (s *composeService) attach(ctx context.Context, project *types.Project, listener api.ContainerEventListener, selectedServices []string) (Containers, error) {
	containers, err := s.getContainers(ctx, project.Name, oneOffExclude, true, selectedServices...)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return containers, nil
	}

	containers.sorted() // This enforces predictable colors assignment

	var names []string
	for _, ctr := range containers {
		names = append(names, getContainerNameWithoutProject(ctr))
	}

	_, err = fmt.Fprintf(s.stdout(), "Attaching to %s\n", strings.Join(names, ", "))
	if err != nil {
		logrus.Debugf("failed to write attach message: %v", err)
	}

	for _, ctr := range containers {
		service := ctr.Labels[api.ServiceLabel]
		err := s.doAttachContainer(ctx, service, ctr.ID, getContainerNameWithoutProject(ctr), listener)
		if err != nil {
			return nil, err
		}
	}
	return containers, nil
}

func (s *composeService) doAttachContainer(ctx context.Context, service, id, name string, listener api.ContainerEventListener) error {
	inspect, err := s.apiClient().ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	wOut := utils.GetWriter(func(line string) {
		listener(api.ContainerEvent{
			Type:    api.ContainerEventLog,
			Source:  name,
			ID:      id,
			Service: service,
			Line:    line,
		})
	})
	wErr := utils.GetWriter(func(line string) {
		listener(api.ContainerEvent{
			Type:    api.ContainerEventErr,
			Source:  name,
			ID:      id,
			Service: service,
			Line:    line,
		})
	})

	err = s.attachContainerStreams(ctx, id, inspect.Container.Config.Tty, wOut, wErr)
	if err != nil {
		return err
	}

	return nil
}

func (s *composeService) attachContainerStreams(ctx context.Context, containerID string, tty bool, stdout, stderr io.WriteCloser) error {
	streamOut, err := s.getContainerStreams(ctx, containerID)
	if err != nil {
		return err
	}

	if stdout != nil {
		go func() {
			defer func() {
				if err := stdout.Close(); err != nil {
					logrus.Debugf("failed to close stdout: %v", err)
				}
				if err := stderr.Close(); err != nil {
					logrus.Debugf("failed to close stderr: %v", err)
				}
				if err := streamOut.Close(); err != nil {
					logrus.Debugf("failed to close stream output: %v", err)
				}
			}()

			var err error
			if tty {
				_, err = io.Copy(stdout, streamOut)
			} else {
				_, err = stdcopy.StdCopy(stdout, stderr, streamOut)
			}
			if err != nil && !errors.Is(err, io.EOF) {
				logrus.Debugf("stream copy error for container %s: %v", containerID, err)
			}
		}()
	}
	return nil
}

func (s *composeService) getContainerStreams(ctx context.Context, containerID string) (io.ReadCloser, error) {
	attachResponse, err := s.apiClient().ContainerAttach(ctx, containerID, client.ContainerAttachOptions{
		Stream: true,
		Stdin:  false,
		Stdout: true,
		Stderr: true,
		Logs:   false,
	})
	if err == nil {
		stdout := ContainerStdout{HijackedResponse: attachResponse.HijackedResponse}
		return stdout, nil
	}

	// Fallback to logs API
	logs, err := s.apiClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, err
	}
	return logs, nil
}
