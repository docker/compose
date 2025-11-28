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
	"slices"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Volumes(ctx context.Context, project string, options api.VolumesOptions) ([]api.VolumesSummary, error) {
	allContainers, err := s.apiClient().ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(projectFilter(project)),
	})
	if err != nil {
		return nil, err
	}

	var containers []container.Summary

	if len(options.Services) > 0 {
		// filter service containers
		for _, c := range allContainers {
			if slices.Contains(options.Services, c.Labels[api.ServiceLabel]) {
				containers = append(containers, c)
			}
		}
	} else {
		containers = allContainers
	}

	volumesResponse, err := s.apiClient().VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(projectFilter(project)),
	})
	if err != nil {
		return nil, err
	}

	projectVolumes := volumesResponse.Volumes

	if len(options.Services) == 0 {
		return projectVolumes, nil
	}

	var volumes []api.VolumesSummary

	// create a name lookup of volumes used by containers
	serviceVolumes := make(map[string]bool)

	for _, container := range containers {
		for _, mount := range container.Mounts {
			serviceVolumes[mount.Name] = true
		}
	}

	// append if volumes in this project are in serviceVolumes
	for _, v := range projectVolumes {
		if serviceVolumes[v.Name] {
			volumes = append(volumes, v)
		}
	}

	return volumes, nil
}
