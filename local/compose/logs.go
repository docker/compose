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
	"bytes"
	"context"
	"io"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/utils"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	list, err := s.apiClient.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
			oneOffFilter(false),
		),
		All: true,
	})

	ignore := func(string) bool {
		return false
	}
	if len(options.Services) > 0 {
		ignore = func(s string) bool {
			return !contains(options.Services, s)
		}
	}

	if err != nil {
		return err
	}
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range list {
		c := c
		service := c.Labels[serviceLabel]
		if ignore(service) {
			continue
		}
		container, err := s.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return err
		}

		eg.Go(func() error {
			r, err := s.apiClient.ContainerLogs(ctx, container.ID, types.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Follow:     options.Follow,
				Tail:       options.Tail,
			})
			defer r.Close() // nolint errcheck

			if err != nil {
				return err
			}
			w := utils.GetWriter(service, getContainerNameWithoutProject(c), consumer)
			if container.Config.Tty {
				_, err = io.Copy(w, r)
			} else {
				_, err = stdcopy.StdCopy(w, w, r)
			}
			return err
		})
	}
	return eg.Wait()
}

type splitBuffer struct {
	buffer    bytes.Buffer
	name      string
	service   string
	container string
	consumer  compose.ContainerEventListener
}

// getWriter creates a io.Writer that will actually split by line and format by LogConsumer
func getWriter(name, service, container string, events compose.ContainerEventListener) io.Writer {
	return &splitBuffer{
		buffer:    bytes.Buffer{},
		name:      name,
		service:   service,
		container: container,
		consumer:  events,
	}
}

// Write implements io.Writer. joins all input, splits on the separator and yields each chunk
func (s *splitBuffer) Write(b []byte) (int, error) {
	n, err := s.buffer.Write(b)
	if err != nil {
		return n, err
	}
	for {
		b = s.buffer.Bytes()
		index := bytes.Index(b, []byte{'\n'})
		if index < 0 {
			break
		}
		line := s.buffer.Next(index + 1)
		s.consumer(compose.ContainerEvent{
			Type:    compose.ContainerEventLog,
			Name:    s.name,
			Service: s.service,
			Source:  s.container,
			Line:    string(line[:len(line)-1]),
		})
	}
	return n, nil
}
