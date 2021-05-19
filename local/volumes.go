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

package local

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/docker/compose-cli/api/volumes"
)

type volumeService struct {
	apiClient client.APIClient
}

func (vs *volumeService) List(ctx context.Context) ([]volumes.Volume, error) {
	l, err := vs.apiClient.VolumeList(ctx, filters.NewArgs())
	if err != nil {
		return []volumes.Volume{}, err
	}

	res := []volumes.Volume{}
	for _, v := range l.Volumes {
		res = append(res, volumes.Volume{
			ID:          v.Name,
			Description: description(v),
		})
	}

	return res, nil
}

func (vs *volumeService) Create(ctx context.Context, name string, options interface{}) (volumes.Volume, error) {
	v, err := vs.apiClient.VolumeCreate(ctx, volume.VolumeCreateBody{
		Driver:     "local",
		DriverOpts: nil,
		Labels:     nil,
		Name:       name,
	})
	if err != nil {
		return volumes.Volume{}, err
	}
	return volumes.Volume{ID: name, Description: description(&v)}, nil
}

func (vs *volumeService) Delete(ctx context.Context, volumeID string, options interface{}) error {
	return vs.apiClient.VolumeRemove(ctx, volumeID, false)
}

func (vs *volumeService) Inspect(ctx context.Context, volumeID string) (volumes.Volume, error) {
	v, err := vs.apiClient.VolumeInspect(ctx, volumeID)
	if err != nil {
		return volumes.Volume{}, err
	}
	return volumes.Volume{ID: volumeID, Description: description(&v)}, nil
}

func description(v *types.Volume) string {
	return fmt.Sprintf("Created %s", v.CreatedAt)
}
