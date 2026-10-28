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

package logstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"

	"github.com/docker/compose/v5/internal/engine"
	"github.com/docker/compose/v5/internal/naming"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// AttachContainers attaches to the containers of the selected services of the
// project, reporting their output to listener, and returns them.
func (s *Service) AttachContainers(ctx context.Context, project *types.Project, listener api.ContainerEventListener, selectedServices []string) ([]container.Summary, error) {
	containers, err := s.backend.Containers(ctx, project.Name, engine.OneOffExclude, true, selectedServices...)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return containers, nil
	}

	// This enforces predictable colors assignment
	sort.Slice(containers, func(i, j int) bool {
		return naming.CanonicalContainerName(containers[i]) < naming.CanonicalContainerName(containers[j])
	})

	var names []string
	for _, ctr := range containers {
		names = append(names, naming.ContainerNameWithoutProject(ctr))
	}

	_, err = fmt.Fprintf(s.backend.CLI().Out(), "Attaching to %s\n", strings.Join(names, ", "))
	if err != nil {
		logrus.Debugf("failed to write attach message: %v", err)
	}

	for _, ctr := range containers {
		service := ctr.Labels[api.ServiceLabel]
		err := s.AttachContainer(ctx, service, ctr.ID, naming.ContainerNameWithoutProject(ctr), listener)
		if err != nil {
			return nil, err
		}
	}
	return containers, nil
}

// AttachContainer attaches to one container, reporting its output to listener.
func (s *Service) AttachContainer(ctx context.Context, service, id, name string, listener api.ContainerEventListener) error {
	inspected, err := s.backend.APIClient().ContainerInspect(ctx, id, client.ContainerInspectOptions{})
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

	err = s.attachContainerStreams(ctx, id, inspected.Container.Config.Tty, wOut, wErr)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) attachContainerStreams(ctx context.Context, containerID string, tty bool, stdout, stderr io.WriteCloser) error {
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

func (s *Service) getContainerStreams(ctx context.Context, containerID string) (io.ReadCloser, error) {
	attachResponse, err := s.backend.APIClient().ContainerAttach(ctx, containerID, client.ContainerAttachOptions{
		Stream: true,
		Stdin:  false,
		Stdout: true,
		Stderr: true,
		Logs:   false,
	})
	if err == nil {
		stdout := hijackedStdout{HijackedResponse: attachResponse.HijackedResponse}
		return stdout, nil
	}

	// Fallback to logs API
	logs, err := s.backend.APIClient().ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return nil, err
	}
	return logs, nil
}

var _ io.ReadCloser = hijackedStdout{}

// hijackedStdout implements ReadCloser for client.HijackedResponse.
type hijackedStdout struct {
	client.HijackedResponse
}

// Read implements io.ReadCloser.
func (l hijackedStdout) Read(p []byte) (n int, err error) {
	return l.Reader.Read(p)
}

// Close implements io.ReadCloser.
func (l hijackedStdout) Close() error {
	l.HijackedResponse.Close()
	return nil
}
