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
	"path/filepath"
	"sort"
	"testing"

	composeloader "github.com/compose-spec/compose-go/v2/loader"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/docker/api/types/network"

	composetypes "github.com/compose-spec/compose-go/v2/types"
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
	assert.DeepEqual(t, project.Networks["skynet"].CustomLabels, composetypes.Labels(map[string]string{
		"com.docker.compose.network": "skynet",
		"com.docker.compose.project": "myProject",
		"com.docker.compose.version": api.ComposeVersion,
	}))
}

func TestBuildContainerMountOptions(t *testing.T) {
	project := composetypes.Project{
		Name: "myProject",
		Services: composetypes.Services{
			"myService": {
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
						Type:   composetypes.VolumeTypeVolume,
						Source: "myVolume3",
						Target: "/var/myvolume3",
						Volume: &composetypes.ServiceVolumeVolume{
							Subpath: "etc",
						},
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

	inherit := &container.Summary{
		Mounts: []container.MountPoint{
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

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mock, cli := prepareMocks(mockCtrl)
	s := composeService{
		dockerCli: cli,
	}
	mock.EXPECT().ImageInspect(gomock.Any(), "myProject-myService").AnyTimes().Return(image.InspectResponse{}, nil)

	mounts, err := s.buildContainerMountOptions(context.TODO(), project, project.Services["myService"], inherit)
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Target < mounts[j].Target
	})
	assert.NilError(t, err)
	assert.Assert(t, len(mounts) == 4)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")
	assert.Equal(t, mounts[2].Target, "/var/myvolume3")
	assert.Equal(t, mounts[2].VolumeOptions.Subpath, "etc")
	assert.Equal(t, mounts[3].Target, "\\\\.\\pipe\\docker_engine")

	mounts, err = s.buildContainerMountOptions(context.TODO(), project, project.Services["myService"], inherit)
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].Target < mounts[j].Target
	})
	assert.NilError(t, err)
	assert.Assert(t, len(mounts) == 4)
	assert.Equal(t, mounts[0].Target, "/var/myvolume1")
	assert.Equal(t, mounts[1].Target, "/var/myvolume2")
	assert.Equal(t, mounts[2].Target, "/var/myvolume3")
	assert.Equal(t, mounts[2].VolumeOptions.Subpath, "etc")
	assert.Equal(t, mounts[3].Target, "\\\\.\\pipe\\docker_engine")
}

func TestDefaultNetworkSettings(t *testing.T) {
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
			Services: composetypes.Services{
				"myService": service,
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

		networkMode, networkConfig, err := defaultNetworkSettings(&project, service, 1, nil, true, "1.43")
		assert.NilError(t, err)
		assert.Equal(t, string(networkMode), "myProject_myNetwork2")
		assert.Check(t, cmp.Len(networkConfig.EndpointsConfig, 1))
		assert.Check(t, cmp.Contains(networkConfig.EndpointsConfig, "myProject_myNetwork2"))
	})

	t.Run("returns default network when service has no networks", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name: "myService",
		}
		project := composetypes.Project{
			Name: "myProject",
			Services: composetypes.Services{
				"myService": service,
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

		networkMode, networkConfig, err := defaultNetworkSettings(&project, service, 1, nil, true, "1.43")
		assert.NilError(t, err)
		assert.Equal(t, string(networkMode), "myProject_default")
		assert.Check(t, cmp.Len(networkConfig.EndpointsConfig, 1))
		assert.Check(t, cmp.Contains(networkConfig.EndpointsConfig, "myProject_default"))
	})

	t.Run("returns none if project has no networks", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name: "myService",
		}
		project := composetypes.Project{
			Name: "myProject",
			Services: composetypes.Services{
				"myService": service,
			},
		}

		networkMode, networkConfig, err := defaultNetworkSettings(&project, service, 1, nil, true, "1.43")
		assert.NilError(t, err)
		assert.Equal(t, string(networkMode), "none")
		assert.Check(t, cmp.Nil(networkConfig))
	})

	t.Run("returns defined network mode if explicitly set", func(t *testing.T) {
		service := composetypes.ServiceConfig{
			Name:        "myService",
			NetworkMode: "host",
		}
		project := composetypes.Project{
			Name:     "myProject",
			Services: composetypes.Services{"myService": service},
			Networks: composetypes.Networks(map[string]composetypes.NetworkConfig{
				"default": {
					Name: "myProject_default",
				},
			}),
		}

		networkMode, networkConfig, err := defaultNetworkSettings(&project, service, 1, nil, true, "1.43")
		assert.NilError(t, err)
		assert.Equal(t, string(networkMode), "host")
		assert.Check(t, cmp.Nil(networkConfig))
	})
}

func TestCreateEndpointSettings(t *testing.T) {
	eps := createEndpointSettings(&composetypes.Project{
		Name: "projName",
	}, composetypes.ServiceConfig{
		Name:          "serviceName",
		ContainerName: "containerName",
		Networks: map[string]*composetypes.ServiceNetworkConfig{
			"netName": {
				Priority:     100,
				Aliases:      []string{"alias1", "alias2"},
				Ipv4Address:  "10.16.17.18",
				Ipv6Address:  "fdb4:7a7f:373a:3f0c::42",
				LinkLocalIPs: []string{"169.254.10.20"},
				MacAddress:   "10:00:00:00:01",
				DriverOpts: composetypes.Options{
					"driverOpt1": "optval1",
					"driverOpt2": "optval2",
				},
			},
		},
	}, 0, "netName", []string{"link1", "link2"}, true)
	assert.Check(t, cmp.DeepEqual(eps, &network.EndpointSettings{
		IPAMConfig: &network.EndpointIPAMConfig{
			IPv4Address:  "10.16.17.18",
			IPv6Address:  "fdb4:7a7f:373a:3f0c::42",
			LinkLocalIPs: []string{"169.254.10.20"},
		},
		Links:      []string{"link1", "link2"},
		Aliases:    []string{"containerName", "serviceName", "alias1", "alias2"},
		MacAddress: "10:00:00:00:01",
		DriverOpts: map[string]string{
			"driverOpt1": "optval1",
			"driverOpt2": "optval2",
		},

		// FIXME(robmry) - IPAddress and IPv6Gateway are "operational data" fields...
		//  - The IPv6 address here is the container's address, not the gateway.
		//  - Both fields will be cleared by the daemon, but they could be removed from
		//    the request.
		IPAddress:   "10.16.17.18",
		IPv6Gateway: "fdb4:7a7f:373a:3f0c::42",
	}))
}

func Test_buildContainerVolumes(t *testing.T) {
	pwd, err := os.Getwd()
	assert.NilError(t, err)

	tests := []struct {
		name   string
		yaml   string
		binds  []string
		mounts []mountTypes.Mount
	}{
		{
			name: "bind mount local path",
			yaml: `
services:
  test:
    volumes:
      - ./data:/data
`,
			binds:  []string{filepath.Join(pwd, "data") + ":/data:rw"},
			mounts: nil,
		},
		{
			name: "bind mount, not create host path",
			yaml: `
services:
  test:
    volumes:
      - type: bind
        source: ./data
        target: /data
        bind:
          create_host_path: false
`,
			binds: nil,
			mounts: []mountTypes.Mount{
				{
					Type:        "bind",
					Source:      filepath.Join(pwd, "data"),
					Target:      "/data",
					BindOptions: &mountTypes.BindOptions{CreateMountpoint: false},
				},
			},
		},
		{
			name: "mount volume",
			yaml: `
services:
  test:
    volumes:
      - data:/data
volumes:
  data:
    name: my_volume
`,
			binds:  []string{"my_volume:/data:rw"},
			mounts: nil,
		},
		{
			name: "mount volume, readonly",
			yaml: `
services:
  test:
    volumes:
      - data:/data:ro
volumes:
  data:
    name: my_volume
`,
			binds:  []string{"my_volume:/data:ro"},
			mounts: nil,
		},
		{
			name: "mount volume subpath",
			yaml: `
services:
  test:
    volumes:
      - type: volume
        source: data
        target: /data
        volume:
          subpath: test/
volumes:
  data: 
    name: my_volume
`,
			binds: nil,
			mounts: []mountTypes.Mount{
				{
					Type:          "volume",
					Source:        "my_volume",
					Target:        "/data",
					VolumeOptions: &mountTypes.VolumeOptions{Subpath: "test/"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := composeloader.LoadWithContext(context.TODO(), composetypes.ConfigDetails{
				ConfigFiles: []composetypes.ConfigFile{
					{
						Filename: "test",
						Content:  []byte(tt.yaml),
					},
				},
			}, func(options *composeloader.Options) {
				options.SkipValidation = true
				options.SkipConsistencyCheck = true
			})
			assert.NilError(t, err)
			s := &composeService{}
			binds, mounts, err := s.buildContainerVolumes(context.TODO(), *p, p.Services["test"], nil)
			assert.NilError(t, err)
			assert.DeepEqual(t, tt.binds, binds)
			assert.DeepEqual(t, tt.mounts, mounts)
		})
	}
}
