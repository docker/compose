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

	"github.com/docker/compose-cli/api/compose"

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types/container"
	"github.com/sirupsen/logrus"
)

func (s *composeService) Start(ctx context.Context, project *types.Project, options compose.StartOptions) error {
	var containers Containers
	if options.Attach != nil {
		c, err := s.attach(ctx, project, options.Attach)
		if err != nil {
			return err
		}
		containers = c
	}

	err := InDependencyOrder(ctx, project, func(c context.Context, service types.ServiceConfig) error {
		return s.startService(ctx, project, service)
	})
	if err != nil {
		return err
	}

	if options.Attach == nil {
		return nil
	}

	for _, c := range containers {
		c := c
		go func() {
			statusC, errC := s.apiClient.ContainerWait(context.Background(), c.ID, container.WaitConditionNotRunning)
			select {
			case status := <-statusC:
				options.Attach <- compose.ContainerEvent{
					Type:     compose.ContainerEventExit,
					Source:   getCanonicalContainerName(c),
					ExitCode: int(status.StatusCode),
				}
			case err := <-errC:
				logrus.Warnf("Unexpected API error for %s : %s\n", getCanonicalContainerName(c), err.Error())
			}
		}()
	}
	return nil
}
