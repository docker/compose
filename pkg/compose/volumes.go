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

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/docker/compose/v5/pkg/api"
)

func (s *composeService) Volumes(ctx context.Context, project string, options api.VolumesOptions) ([]api.VolumesSummary, error) {
	allContainers, err := s.apiClient().ContainerList(ctx, client.ContainerListOptions{
		Filters: projectFilter(project),
	})
	if err != nil {
		return nil, err
	}

	var containers []container.Summary

	if len(options.Services) > 0 {
		// filter service containers
		for _, c := range allContainers.Items {
			if slices.Contains(options.Services, c.Labels[api.ServiceLabel]) {
				containers = append(containers, c)
			}
		}
	} else {
		containers = allContainers.Items
	}

	volumesResponse, err := s.apiClient().VolumeList(ctx, client.VolumeListOptions{
		Filters: projectFilter(project),
	})
	if err != nil {
		return nil, err
	}

	projectVolumes := volumesResponse.Items

	if len(options.Services) == 0 {
		return projectVolumes, nil
	}

	var volumes []api.VolumesSummary

	// create a name lookup of volumes used by containers
	serviceVolumes := make(map[string]bool)

	for _, ctr := range containers {
		for _, mount := range ctr.Mounts {
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
