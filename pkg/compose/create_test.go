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
	"os"
	"path"
	"sort"
	"testing"

	"github.com/compose-spec/compose-go/types"
	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	mountTypes "github.com/docker/docker/api/types/mount"
	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"
)

func TestBuildContainerVolumesBindsFromVolumes(t *testing.T) {
	testBuildContainerVolumesBindsFromVolumes(t, "/data")
	testBuildContainerVolumesBindsFromVolumes(t, "/data/")
}

func TestBuildContainerVolumesMountsFromVolumes(t *testing.T) {
	testBuildContainerVolumesMountsFromVolumes(t, "/data")
	testBuildContainerVolumesMountsFromVolumes(t, "/data/")
}

func TestBuildContainerVolumesMountsFromImageInherited(t *testing.T) {
	testBuildContainerVolumesMountsFromImageInherited(t, "/data")
	testBuildContainerVolumesMountsFromImageInherited(t, "/data/")
}

func TestBuildContainerVolumesMountsFromVolumeInherited(t *testing.T) {
	testBuildContainerVolumesMountsFromVolumeInherited(t, "/data")
	testBuildContainerVolumesMountsFromVolumeInherited(t, "/data/")
}

func TestBuildContainerVolumesMountsFromConfig(t *testing.T) {
	testBuildContainerVolumesMountsFromConfig(t, "/data")
	testBuildContainerVolumesMountsFromConfig(t, "/data/")
}

func TestBuildContainerVolumesMountsFromSecret(t *testing.T) {
	testBuildContainerVolumesMountsFromSecret(t, "/data")
	testBuildContainerVolumesMountsFromSecret(t, "/data/")
}

func testBuildContainerVolumesBindsFromVolumes(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{},
		[]composetypes.ServiceVolumeConfig{
			{
				Type:   composetypes.VolumeTypeBind,
				Source: "",
				Target: target,
				Bind: &composetypes.ServiceVolumeBind{
					CreateHostPath: true,
				},
			},
		}, nil, nil, nil)
	assert.NilError(t, err)

	source, err := os.Getwd()
	assert.NilError(t, err)
	assert.DeepEqual(t, []string{
		source + ":/data:rw",
	}, binds)
	assert.Equal(t, 0, len(mounts))
}

func testBuildContainerVolumesMountsFromVolumes(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{},
		[]composetypes.ServiceVolumeConfig{
			{
				Type:   composetypes.VolumeTypeBind,
				Source: "",
				Target: target,
			},
		}, nil, nil, nil)
	assert.NilError(t, err)

	assert.Equal(t, 0, len(binds))
	source, err := os.Getwd()
	assert.NilError(t, err)
	assert.DeepEqual(t, []mountTypes.Mount{
		{
			Type:   mountTypes.TypeBind,
			Source: source,
			Target: "/data",
		},
	}, mounts)
}

func testBuildContainerVolumesMountsFromImageInherited(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{
			Config: &container.Config{
				Volumes: map[string]struct{}{
					path.Clean(target): {}},
			},
		},
		nil,
		nil,
		nil,
		&moby.Container{
			Mounts: []moby.MountPoint{
				{
					Type:        mountTypes.TypeBind,
					Source:      "source",
					Destination: target,
					RW:          true,
				},
			},
		})
	assert.NilError(t, err)

	assert.Equal(t, 0, len(binds))
	assert.DeepEqual(t, []mountTypes.Mount{
		{
			Type:   mountTypes.TypeBind,
			Source: "source",
			Target: "/data",
		},
	}, mounts)
}

func testBuildContainerVolumesMountsFromVolumeInherited(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{},
		[]types.ServiceVolumeConfig{
			{
				Type:   composetypes.VolumeTypeBind,
				Target: target,
			},
		},
		nil,
		nil,
		&moby.Container{
			Mounts: []moby.MountPoint{
				{
					Type:        mountTypes.TypeBind,
					Source:      "source",
					Destination: path.Clean(target),
					RW:          true,
				},
			},
		})
	assert.NilError(t, err)

	assert.Equal(t, 0, len(binds))
	assert.DeepEqual(t, []mountTypes.Mount{
		{
			Type:   mountTypes.TypeBind,
			Source: "source",
			Target: "/data",
		},
	}, mounts)
}

func testBuildContainerVolumesMountsFromConfig(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{},
		[]types.ServiceVolumeConfig{
			{
				Type:   composetypes.VolumeTypeBind,
				Target: target,
			},
		},
		[]types.ServiceConfigObjConfig{
			{
				Target: target,
			},
		}, nil, nil)
	assert.NilError(t, err)

	assert.Equal(t, 0, len(binds))
	source, err := os.Getwd()
	assert.NilError(t, err)
	assert.DeepEqual(t, []mountTypes.Mount{
		{
			Type:   mountTypes.TypeBind,
			Source: source,
			Target: "/data",
		},
	}, mounts)
}

func testBuildContainerVolumesMountsFromSecret(t *testing.T, target string) {
	binds, mounts, err := testBuildContainerVolumes(t,
		moby.ImageInspect{},
		[]types.ServiceVolumeConfig{
			{
				Type:   composetypes.VolumeTypeBind,
				Target: target,
			},
		},
		nil,
		[]types.ServiceSecretConfig{
			{
				Target: target,
			},
		}, nil)
	assert.NilError(t, err)

	assert.Equal(t, 0, len(binds))
	source, err := os.Getwd()
	assert.NilError(t, err)
	assert.DeepEqual(t, []mountTypes.Mount{
		{
			Type:   mountTypes.TypeBind,
			Source: source,
			Target: "/data",
		},
	}, mounts)
}

func testBuildContainerVolumes(
	t *testing.T,
	image moby.ImageInspect,
	volumes []composetypes.ServiceVolumeConfig,
	configs []composetypes.ServiceConfigObjConfig,
	secrets []composetypes.ServiceSecretConfig,
	inherit *moby.Container) ([]string, []mountTypes.Mount, error) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockAPI := mocks.NewMockAPIClient(mockCtrl)
	ctx := context.Background()
	mockAPI.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(image, nil, nil)

	s := &composeService{apiClient: mockAPI}
	volumeMounts, binds, mounts, err := s.buildContainerVolumes(ctx,
		composetypes.Project{},
		composetypes.ServiceConfig{
			Volumes: volumes,
			Configs: configs,
			Secrets: secrets,
		}, inherit)

	assert.NilError(t, err)
	assert.DeepEqual(t, map[string]struct{}{
		"/data": {},
	}, volumeMounts)
	return binds, mounts, err
}

func TestBuildVolumeMount(t *testing.T) {
	project := composetypes.Project{
		Name: "myProject",
		Volumes: composetypes.Volumes(map[string]composetypes.VolumeConfig{
			"myVolume": {
				Name: "myProject_myVolume",
			},
		}),
	}
	volume := composetypes.ServiceVolumeConfig{
		Type:   composetypes.VolumeTypeVolume,
		Source: "myVolume",
		Target: "/data",
	}
	mount, err := buildMount(project, volume)
	assert.NilError(t, err)
	assert.Equal(t, mount.Source, "myProject_myVolume")
	assert.Equal(t, mount.Type, mountTypes.TypeVolume)
}

func TestServiceImageName(t *testing.T) {
	assert.Equal(t, getImageName(types.ServiceConfig{Image: "myImage"}, "myProject"), "myImage")
	assert.Equal(t, getImageName(types.ServiceConfig{Name: "aService"}, "myProject"), "myProject_aService")
}

func TestPrepareNetworkLabels(t *testing.T) {
	project := types.Project{
		Name:     "myProject",
		Networks: types.Networks(map[string]types.NetworkConfig{"skynet": {}}),
	}
	prepareNetworks(&project)
	assert.DeepEqual(t, project.Networks["skynet"].Labels, types.Labels(map[string]string{
		"com.docker.compose.network": "skynet",
		"com.docker.compose.project": "myProject",
		"com.docker.compose.version": api.ComposeVersion,
	}))
}

func TestBuildContainerMountOptions(t *testing.T) {
	project := composetypes.Project{
		Name: "myProject",
		Services: []composetypes.ServiceConfig{
			{
				Name: "myService",
				Volumes: []composetypes.ServiceVolumeConfig{
					{
						Type:   composetypes.VolumeTypeVolume,
						Target: "/var/myvolume1",
					},
					{
						Type:   composetypes.VolumeTypeVolume,
						Target: "/var/myvolume2",
					},
				},
			},
		},
		Volumes: composetypes.Volumes(map[string]composetypes.VolumeConfig{
			"myVolume1": {
				Name: "myProject_myVolume1",
			},
			"myVolume2": {
				Name: "myProject_myVolume2",
			},
		}),
	}

	inherit := &moby.Container{
		Mounts: []moby.MountPoint{
			{
				Type:        composetypes.VolumeTypeVolume,
				Destination: "/var/myvolume1",
			},
			{
				Type:        composetypes.VolumeTypeVolume,
				Destination: "/var/myvolume2",
			},
		},
	}

	mounts, err := buildContainerMountOptions(project, project.Services[0], moby.ImageInspect{}, inherit)
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Target < mounts[j].Target
	})
	assert.NilError(t, err)
	assert.Assert(t, len(mounts) == 2)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")

	mounts, err = buildContainerMountOptions(project, project.Services[0], moby.ImageInspect{}, inherit)
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Target < mounts[j].Target
	})
	assert.NilError(t, err)
	assert.Assert(t, len(mounts) == 2)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")
}
