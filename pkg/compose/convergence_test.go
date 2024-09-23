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
	"fmt"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
	"github.com/docker/compose/v2/pkg/progress"
)

func TestContainerName(t *testing.T) {
	s := types.ServiceConfig{
		Name:          "testservicename",
		ContainerName: "testcontainername",
		Scale:         intPtr(1),
		Deploy:        &types.DeployConfig{},
	}
	ret, err := getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, *s.Scale)

	s.Scale = intPtr(0)
	ret, err = getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, *s.Scale)

	s.Scale = intPtr(2)
	_, err = getScale(s)
	assert.Error(t, err, fmt.Sprintf(doubledContainerNameWarning, s.Name, s.ContainerName))
}

func intPtr(i int) *int {
	return &i
}

func TestServiceLinks(t *testing.T) {
	const dbContainerName = "/" + testProject + "-db-1"
	const webContainerName = "/" + testProject + "-web-1"
	s := types.ServiceConfig{
		Name:  "web",
		Scale: intPtr(1),
	}

	containerListOptions := containerType.ListOptions{
		Filters: filters.NewArgs(
			projectFilter(testProject),
			serviceFilter("db"),
			oneOffFilter(false),
			hasConfigHashLabel(),
		),
		All: true,
	}

	t.Run("service links default", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return([]moby.Container{c}, nil)

		links, err := tested.getLinks(context.Background(), testProject, s, 1)
		assert.NilError(t, err)

		assert.Equal(t, len(links), 3)
		assert.Equal(t, links[0], "testProject-db-1:db")
		assert.Equal(t, links[1], "testProject-db-1:db-1")
		assert.Equal(t, links[2], "testProject-db-1:testProject-db-1")
	})

	t.Run("service links", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:db"}

		c := testContainer("db", dbContainerName, false)

		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return([]moby.Container{c}, nil)
		links, err := tested.getLinks(context.Background(), testProject, s, 1)
		assert.NilError(t, err)

		assert.Equal(t, len(links), 3)
		assert.Equal(t, links[0], "testProject-db-1:db")
		assert.Equal(t, links[1], "testProject-db-1:db-1")
		assert.Equal(t, links[2], "testProject-db-1:testProject-db-1")
	})

	t.Run("service links name", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:dbname"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return([]moby.Container{c}, nil)

		links, err := tested.getLinks(context.Background(), testProject, s, 1)
		assert.NilError(t, err)

		assert.Equal(t, len(links), 3)
		assert.Equal(t, links[0], "testProject-db-1:dbname")
		assert.Equal(t, links[1], "testProject-db-1:db-1")
		assert.Equal(t, links[2], "testProject-db-1:testProject-db-1")
	})

	t.Run("service links external links", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:dbname"}
		s.ExternalLinks = []string{"db1:db2"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return([]moby.Container{c}, nil)

		links, err := tested.getLinks(context.Background(), testProject, s, 1)
		assert.NilError(t, err)

		assert.Equal(t, len(links), 4)
		assert.Equal(t, links[0], "testProject-db-1:dbname")
		assert.Equal(t, links[1], "testProject-db-1:db-1")
		assert.Equal(t, links[2], "testProject-db-1:testProject-db-1")

		// ExternalLink
		assert.Equal(t, links[3], "db1:db2")
	})

	t.Run("service links itself oneoff", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{}
		s.ExternalLinks = []string{}
		s.Labels = s.Labels.Add(api.OneoffLabel, "True")

		c := testContainer("web", webContainerName, true)
		containerListOptionsOneOff := containerType.ListOptions{
			Filters: filters.NewArgs(
				projectFilter(testProject),
				serviceFilter("web"),
				oneOffFilter(false),
				hasConfigHashLabel(),
			),
			All: true,
		}
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptionsOneOff).Return([]moby.Container{c}, nil)

		links, err := tested.getLinks(context.Background(), testProject, s, 1)
		assert.NilError(t, err)

		assert.Equal(t, len(links), 3)
		assert.Equal(t, links[0], "testProject-web-1:web")
		assert.Equal(t, links[1], "testProject-web-1:web-1")
		assert.Equal(t, links[2], "testProject-web-1:testProject-web-1")
	})
}

func TestWaitDependencies(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	t.Run("should skip dependencies with scale 0", func(t *testing.T) {
		dbService := types.ServiceConfig{Name: "db", Scale: intPtr(0)}
		redisService := types.ServiceConfig{Name: "redis", Scale: intPtr(0)}
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db":    dbService,
			"redis": redisService,
		}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: ServiceConditionRunningOrHealthy},
			"redis": {Condition: ServiceConditionRunningOrHealthy},
		}
		assert.NilError(t, tested.waitDependencies(context.Background(), &project, "", dependencies, nil, 0))
	})
	t.Run("should skip dependencies with condition service_started", func(t *testing.T) {
		dbService := types.ServiceConfig{Name: "db", Scale: intPtr(1)}
		redisService := types.ServiceConfig{Name: "redis", Scale: intPtr(1)}
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db":    dbService,
			"redis": redisService,
		}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: types.ServiceConditionStarted, Required: true},
			"redis": {Condition: types.ServiceConditionStarted, Required: true},
		}
		assert.NilError(t, tested.waitDependencies(context.Background(), &project, "", dependencies, nil, 0))
	})
}

func TestCreateMobyContainer(t *testing.T) {
	t.Run("connects container networks one by one if API <1.44", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
		apiClient.EXPECT().DaemonHost().Return("").AnyTimes()
		apiClient.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(moby.ImageInspect{}, nil, nil).AnyTimes()
		// force `RuntimeVersion` to fetch again
		runtimeVersion = runtimeVersionCache{}
		apiClient.EXPECT().ServerVersion(gomock.Any()).Return(moby.Version{
			APIVersion: "1.43",
		}, nil).AnyTimes()

		service := types.ServiceConfig{
			Name: "test",
			Networks: map[string]*types.ServiceNetworkConfig{
				"a": {
					Priority: 10,
				},
				"b": {
					Priority: 100,
				},
			},
		}
		project := types.Project{
			Name: "bork",
			Services: types.Services{
				"test": service,
			},
			Networks: types.Networks{
				"a": types.NetworkConfig{
					Name: "a-moby-name",
				},
				"b": types.NetworkConfig{
					Name: "b-moby-name",
				},
			},
		}

		var falseBool bool
		apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Eq(
			&containerType.HostConfig{
				PortBindings: nat.PortMap{},
				ExtraHosts:   []string{},
				Tmpfs:        map[string]string{},
				Resources: containerType.Resources{
					OomKillDisable: &falseBool,
				},
				NetworkMode: "b-moby-name",
			}), gomock.Eq(
			&network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					"b-moby-name": {
						IPAMConfig: &network.EndpointIPAMConfig{},
						Aliases:    []string{"bork-test-0"},
					},
				},
			}), gomock.Any(), gomock.Any()).Times(1).Return(
			containerType.CreateResponse{
				ID: "an-id",
			}, nil)

		apiClient.EXPECT().ContainerInspect(gomock.Any(), gomock.Eq("an-id")).Times(1).Return(
			moby.ContainerJSON{
				ContainerJSONBase: &moby.ContainerJSONBase{
					ID:   "an-id",
					Name: "a-name",
				},
				Config:          &containerType.Config{},
				NetworkSettings: &moby.NetworkSettings{},
			}, nil)

		apiClient.EXPECT().NetworkConnect(gomock.Any(), "a-moby-name", "an-id", gomock.Eq(
			&network.EndpointSettings{
				IPAMConfig: &network.EndpointIPAMConfig{},
				Aliases:    []string{"bork-test-0"},
			}))

		_, err := tested.createMobyContainer(context.Background(), &project, service, "test", 0, nil, createOptions{
			Labels: make(types.Labels),
		}, progress.ContextWriter(context.TODO()))
		assert.NilError(t, err)
	})

	t.Run("includes all container networks in ContainerCreate call if API >=1.44", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		tested := composeService{
			dockerCli: cli,
		}
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
		apiClient.EXPECT().DaemonHost().Return("").AnyTimes()
		apiClient.EXPECT().ImageInspectWithRaw(gomock.Any(), gomock.Any()).Return(moby.ImageInspect{}, nil, nil).AnyTimes()
		// force `RuntimeVersion` to fetch fresh version
		runtimeVersion = runtimeVersionCache{}
		apiClient.EXPECT().ServerVersion(gomock.Any()).Return(moby.Version{
			APIVersion: "1.44",
		}, nil).AnyTimes()

		service := types.ServiceConfig{
			Name: "test",
			Networks: map[string]*types.ServiceNetworkConfig{
				"a": {
					Priority: 10,
				},
				"b": {
					Priority: 100,
				},
			},
		}
		project := types.Project{
			Name: "bork",
			Services: types.Services{
				"test": service,
			},
			Networks: types.Networks{
				"a": types.NetworkConfig{
					Name: "a-moby-name",
				},
				"b": types.NetworkConfig{
					Name: "b-moby-name",
				},
			},
		}

		var falseBool bool
		apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any(), gomock.Eq(
			&containerType.HostConfig{
				PortBindings: nat.PortMap{},
				ExtraHosts:   []string{},
				Tmpfs:        map[string]string{},
				Resources: containerType.Resources{
					OomKillDisable: &falseBool,
				},
				NetworkMode: "b-moby-name",
			}), gomock.Eq(
			&network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					"a-moby-name": {
						IPAMConfig: &network.EndpointIPAMConfig{},
						Aliases:    []string{"bork-test-0"},
					},
					"b-moby-name": {
						IPAMConfig: &network.EndpointIPAMConfig{},
						Aliases:    []string{"bork-test-0"},
					},
				},
			}), gomock.Any(), gomock.Any()).Times(1).Return(
			containerType.CreateResponse{
				ID: "an-id",
			}, nil)

		apiClient.EXPECT().ContainerInspect(gomock.Any(), gomock.Eq("an-id")).Times(1).Return(
			moby.ContainerJSON{
				ContainerJSONBase: &moby.ContainerJSONBase{
					ID:   "an-id",
					Name: "a-name",
				},
				Config:          &containerType.Config{},
				NetworkSettings: &moby.NetworkSettings{},
			}, nil)

		_, err := tested.createMobyContainer(context.Background(), &project, service, "test", 0, nil, createOptions{
			Labels: make(types.Labels),
		}, progress.ContextWriter(context.TODO()))
		assert.NilError(t, err)
	})

}
