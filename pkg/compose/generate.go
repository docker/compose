/*
   Copyright 2023 Docker Compose CLI authors

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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

func (s *composeService) Generate(ctx context.Context, options api.ReverseOptions) (*types.Project, error) {
	if options.Project == nil {
		options.Project = &types.Project{}
	}
	filtersListNames := filters.NewArgs()
	filtersListIDs := filters.NewArgs()
	for _, containerName := range options.Containers {
		filtersListNames.Add("name", containerName)
		filtersListIDs.Add("id", containerName)
	}
	containers, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filtersListNames,
		All:     true,
	})
	if err != nil {
		return nil, err
	}

	containersByIds, err := s.apiClient().ContainerList(ctx, containerType.ListOptions{
		Filters: filtersListIDs,
		All:     true,
	})
	if err != nil {
		return nil, err
	}
	for _, container := range containersByIds {
		if !utils.Contains(containers, container) {
			containers = append(containers, container)
		}
	}

	return s.projectFromName(containers, options.Project.Name)
}
