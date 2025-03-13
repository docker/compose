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
	"strings"

	"github.com/docker/compose/v2/pkg/api"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Top(ctx context.Context, projectName string, services []string) ([]api.ContainerProcSummary, error) {
	projectName = strings.ToLower(projectName)
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
	for i, ctr := range containers {
		eg.Go(func() error {
			topContent, err := s.apiClient().ContainerTop(ctx, ctr.ID, []string{})
			if err != nil {
				return err
			}
			name := getCanonicalContainerName(ctr)
			s := api.ContainerProcSummary{
				ID:        ctr.ID,
				Name:      name,
				Processes: topContent.Processes,
				Titles:    topContent.Titles,
				Service:   name,
			}
			if service, exists := ctr.Labels[api.ServiceLabel]; exists {
				s.Service = service
			}
			if replica, exists := ctr.Labels[api.ContainerNumberLabel]; exists {
				s.Replica = replica
			}
			summary[i] = s
			return nil
		})
	}
	return summary, eg.Wait()
}
