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
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/docker/compose/v2/pkg/api"

	composetypes "github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	mountTypes "github.com/docker/docker/api/types/mount"

	"gotest.tools/v3/assert"
)

func TestBuildBindMount(t *testing.T) {
	project := composetypes.Project{}
	volume := composetypes.ServiceVolumeConfig{
		Type:   composetypes.VolumeTypeBind,
		Source: "",
		Target: "/data",
	}
	mount, err := buildMount(project, volume)
	assert.NilError(t, err)
	assert.Assert(t, filepath.IsAbs(mount.Source))
	_, err = os.Stat(mount.Source)
	assert.NilError(t, err)
	assert.Equal(t, mount.Type, mountTypes.TypeBind)
}

func TestBuildNamedPipeMount(t *testing.T) {
	project := composetypes.Project{}
	volume := composetypes.ServiceVolumeConfig{
		Type:   composetypes.VolumeTypeNamedPipe,
		Source: "\\\\.\\pipe\\docker_engine_windows",
		Target: "\\\\.\\pipe\\docker_engine",
	}
	mount, err := buildMount(project, volume)
	assert.NilError(t, err)
	assert.Equal(t, mount.Type, mountTypes.TypeNamedPipe)
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
	assert.Equal(t, api.GetImageNameOrDefault(composetypes.ServiceConfig{Image: "myImage"}, "myProject"), "myImage")
	assert.Equal(t, api.GetImageNameOrDefault(composetypes.ServiceConfig{Name: "aService"}, "myProject"), "myProject-aService")
}

func TestPrepareNetworkLabels(t *testing.T) {
	project := composetypes.Project{
		Name:     "myProject",
		Networks: composetypes.Networks(map[string]composetypes.NetworkConfig{"skynet": {}}),
	}
	prepareNetworks(&project)
	assert.DeepEqual(t, project.Networks["skynet"].Labels, composetypes.Labels(map[string]string{
		"com.docker.compose.network": "skynet",
		"com.docker.compose.project": "myProject",
		"com.docker.compose.version": api.ComposeVersion,
	}))
}

func TestPrepareVolumes(t *testing.T) {
	t.Run("adds dependency condition if service depends on volume from another service", func(t *testing.T) {
		project := composetypes.Project{
			Name: "myProject",
			Services: []composetypes.ServiceConfig{
				{
					Name:        "aService",
					VolumesFrom: []string{"anotherService"},
				},
				{
					Name: "anotherService",
				},
			},
		}
		err := prepareVolumes(&project)
		assert.NilError(t, err)
		assert.Equal(t, project.Services[0].DependsOn["anotherService"].Condition, composetypes.ServiceConditionStarted)
	})
	t.Run("doesn't overwrite existing dependency condition", func(t *testing.T) {
		project := composetypes.Project{
			Name: "myProject",
			Services: []composetypes.ServiceConfig{
				{
					Name:        "aService",
					VolumesFrom: []string{"anotherService"},
					DependsOn: map[string]composetypes.ServiceDependency{
						"anotherService": {Condition: composetypes.ServiceConditionHealthy},
					},
				},
				{
					Name: "anotherService",
				},
			},
		}
		err := prepareVolumes(&project)
		assert.NilError(t, err)
		assert.Equal(t, project.Services[0].DependsOn["anotherService"].Condition, composetypes.ServiceConditionHealthy)
	})
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
					{
						Type:   composetypes.VolumeTypeNamedPipe,
						Source: "\\\\.\\pipe\\docker_engine_windows",
						Target: "\\\\.\\pipe\\docker_engine",
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
	assert.Assert(t, len(mounts) == 3)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")
	assert.Equal(t, mounts[2].Target, "\\\\.\\pipe\\docker_engine")

	mounts, err = buildContainerMountOptions(project, project.Services[0], moby.ImageInspect{}, inherit)
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Target < mounts[j].Target
	})
	assert.NilError(t, err)
	assert.Assert(t, len(mounts) == 3)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")
	assert.Equal(t, mounts[2].Target, "\\\\.\\pipe\\docker_engine")
}

func TestGetDefaultNetworkMode(t *testing.T) {
	t.Run("returns the network with the highest priority when service has multiple networks", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name: "myService",
			Networks: map[string]*composetypes.ServiceNetworkConfig{
				"myNetwork1": {
					Priority: 10,
				},
				"myNetwork2": {
					Priority: 1000,
				},
			},
		}
		project := composetypes.Project{
			Name: "myProject",
			Services: []composetypes.ServiceConfig{
				service,
			},
			Networks: composetypes.Networks(map[string]composetypes.NetworkConfig{
				"myNetwork1": {
					Name: "myProject_myNetwork1",
				},
				"myNetwork2": {
					Name: "myProject_myNetwork2",
				},
			}),
		}

		assert.Equal(t, getDefaultNetworkMode(&project, service), "myProject_myNetwork2")
	})

	t.Run("returns default network when service has no networks", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name: "myService",
		}
		project := composetypes.Project{
			Name: "myProject",
			Services: []composetypes.ServiceConfig{
				service,
			},
			Networks: composetypes.Networks(map[string]composetypes.NetworkConfig{
				"myNetwork1": {
					Name: "myProject_myNetwork1",
				},
				"myNetwork2": {
					Name: "myProject_myNetwork2",
				},
				"default": {
					Name: "myProject_default",
				},
			}),
		}

		assert.Equal(t, getDefaultNetworkMode(&project, service), "myProject_default")
	})

	t.Run("returns none if project has no networks", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name: "myService",
		}
		project := composetypes.Project{
			Name: "myProject",
			Services: []composetypes.ServiceConfig{
				service,
			},
		}

		assert.Equal(t, getDefaultNetworkMode(&project, service), "none")
	})
}
