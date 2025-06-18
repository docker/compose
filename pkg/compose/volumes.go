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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

func (s *composeService) Volumes( ctx context.Context, project *types.Project, options api.VolumesOptions) ([]api.VolumesSummary, error) {

	projectName := project.Name

	volumesResponse, err := s.apiClient().VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(projectFilter(projectName)),
	})
	if err != nil {
		return nil, err
	}

	projectVolumes := volumesResponse.Volumes

	return projectVolumes, nil
}
