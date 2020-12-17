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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Logs(ctx context.Context, projectName string, consumer compose.LogConsumer, options compose.LogOptions) error {
	list, err := s.apiClient.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(projectName),
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
				Follow:     true,
			})
			defer r.Close() // nolint errcheck

			if err != nil {
				return err
			}
			w := getWriter(service, container.ID, consumer)
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
	service   string
	container string
	consumer  compose.LogConsumer
}

// getWriter creates a io.Writer that will actually split by line and format by LogConsumer
func getWriter(service, container string, l compose.LogConsumer) io.Writer {
	return splitBuffer{
		service:   service,
		container: container,
		consumer:  l,
	}
}

func (s splitBuffer) Write(b []byte) (n int, err error) {
	split := bytes.Split(b, []byte{'\n'})
	for _, line := range split {
		if len(line) != 0 {
			s.consumer.Log(s.service, s.container, string(line))
		}
	}
	return len(b), nil
}
