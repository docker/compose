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
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestVolumes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockApi, mockCli := prepareMocks(mockCtrl)
	tested := composeService{
		dockerCli: mockCli,
	}

	// Create test volumes
	vol1 := volume.Volume{Name: testProject + "_vol1"}
	vol2 := volume.Volume{Name: testProject + "_vol2"}
	vol3 := volume.Volume{Name: testProject + "_vol3"}

	// Create test containers with volume mounts
	c1 := container.Summary{
		Labels: map[string]string{api.ServiceLabel: "service1"},
		Mounts: []container.MountPoint{
			{Name: testProject + "_vol1"},
			{Name: testProject + "_vol2"},
		},
	}
	c2 := container.Summary{
		Labels: map[string]string{api.ServiceLabel: "service2"},
		Mounts: []container.MountPoint{
			{Name: testProject + "_vol3"},
		},
	}

	listOpts := client.ContainerListOptions{Filters: projectFilter(testProject)}
	volumeListOpts := client.VolumeListOptions{Filters: projectFilter(testProject)}
	volumeReturn := client.VolumeListResult{
		Items: []volume.Volume{vol1, vol2, vol3},
	}
	containerReturn := client.ContainerListResult{
		Items: []container.Summary{c1, c2},
	}

	mockApi.EXPECT().ContainerList(t.Context(), listOpts).Times(2).Return(containerReturn, nil)
	mockApi.EXPECT().VolumeList(t.Context(), volumeListOpts).Times(2).Return(volumeReturn, nil)

	// Test without service filter - should return all project volumes
	volumes, err := tested.Volumes(t.Context(), testProject, api.VolumesOptions{})
	expected := []api.VolumesSummary{vol1, vol2, vol3}
	assert.NilError(t, err)
	assert.DeepEqual(t, volumes, expected)

	// Test with service filter - should only return volumes used by service1
	volumes, err = tested.Volumes(t.Context(), testProject, api.VolumesOptions{Services: []string{"service1"}})
	expected = []api.VolumesSummary{vol1, vol2}
	assert.NilError(t, err)
	assert.DeepEqual(t, volumes, expected)
}
