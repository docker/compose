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

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"

	"github.com/compose-spec/compose-go/types"
	"golang.org/x/sync/errgroup"
)

func (s *composeService) Stop(ctx context.Context, project *types.Project, consumer compose.LogConsumer) error {
	eg, _ := errgroup.WithContext(ctx)
	w := progress.ContextWriter(ctx)

	var containers Containers
	containers, err := s.apiClient.ContainerList(ctx, moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(project.Name)),
		All:     true,
	})
	if err != nil {
		return err
	}

	err = InReverseDependencyOrder(ctx, project, func(c context.Context, service types.ServiceConfig) error {
		serviceContainers, others := containers.split(isService(service.Name))
		err := s.stopContainers(ctx, w, serviceContainers)
		containers = others
		return err
	})

	return eg.Wait()
}
