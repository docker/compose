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

	"github.com/docker/compose/v2/pkg/api"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Top(ctx context.Context, projectName string, services []string) ([]api.ContainerProcSummary, error) {
	var containers Containers
	containers, err := s.getContainers(ctx, projectName, oneOffInclude, false)
	if err != nil {
		return nil, err
	}
	if len(services) > 0 {
		containers = containers.filter(isService(services...))
	}
	summary := make([]api.ContainerProcSummary, len(containers))
	eg, ctx := errgroup.WithContext(ctx)
	for i, container := range containers {
		i, container := i, container
		eg.Go(func() error {
			topContent, err := s.apiClient.ContainerTop(ctx, container.ID, []string{})
			if err != nil {
				return err
			}
			summary[i] = api.ContainerProcSummary{
				ID:        container.ID,
				Name:      getCanonicalContainerName(container),
				Processes: topContent.Processes,
				Titles:    topContent.Titles,
			}
			return nil
		})
	}
	return summary, eg.Wait()
}
