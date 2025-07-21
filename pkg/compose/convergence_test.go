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
	"net/netip"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
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

	containerListOptions := client.ContainerListOptions{
		Filters: projectFilter(testProject).Add("label",
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
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return(client.ContainerListResult{
			Items: []container.Summary{c},
		}, nil)

		links, err := tested.(*composeService).getLinks(t.Context(), testProject, s, 1)
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
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:db"}

		c := testContainer("db", dbContainerName, false)

		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return(client.ContainerListResult{
			Items: []container.Summary{c},
		}, nil)
		links, err := tested.(*composeService).getLinks(t.Context(), testProject, s, 1)
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
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:dbname"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return(client.ContainerListResult{
			Items: []container.Summary{c},
		}, nil)

		links, err := tested.(*composeService).getLinks(t.Context(), testProject, s, 1)
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
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{"db:dbname"}
		s.ExternalLinks = []string{"db1:db2"}

		c := testContainer("db", dbContainerName, false)
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptions).Return(client.ContainerListResult{
			Items: []container.Summary{c},
		}, nil)

		links, err := tested.(*composeService).getLinks(t.Context(), testProject, s, 1)
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
		tested, err := NewComposeService(cli)
		assert.NilError(t, err)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()

		s.Links = []string{}
		s.ExternalLinks = []string{}
		s.Labels = s.Labels.Add(api.OneoffLabel, "True")

		c := testContainer("web", webContainerName, true)
		containerListOptionsOneOff := client.ContainerListOptions{
			Filters: projectFilter(testProject).Add("label",
				serviceFilter("web"),
				oneOffFilter(false),
				hasConfigHashLabel(),
			),
			All: true,
		}
		apiClient.EXPECT().ContainerList(gomock.Any(), containerListOptionsOneOff).Return(client.ContainerListResult{
			Items: []container.Summary{c},
		}, nil)

		links, err := tested.(*composeService).getLinks(t.Context(), testProject, s, 1)
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
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
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
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "", dependencies, nil, 0))
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
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "", dependencies, nil, 0))
	})
}

func TestIsServiceHealthy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	ctx := t.Context()

	t.Run("disabled healthcheck with fallback to running", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck (Test: ["NONE"])
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with disabled healthcheck should be considered healthy when running with fallbackRunning=true")
	})

	t.Run("disabled healthcheck without fallback", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck (Test: ["NONE"]) but fallbackRunning=false
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		_, err := tested.(*composeService).isServiceHealthy(ctx, containers, false)
		assert.ErrorContains(t, err, "has no healthcheck configured")
	})

	t.Run("no healthcheck with fallback to running", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with no healthcheck at all
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    containerID,
				Name:  "test-container",
				State: &container.State{Status: "running"},
				Config: &container.Config{
					Healthcheck: nil,
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with no healthcheck should be considered healthy when running with fallbackRunning=true")
	})

	t.Run("exited container with disabled healthcheck", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with disabled healthcheck but exited
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:   containerID,
				Name: "test-container",
				State: &container.State{
					Status:   "exited",
					ExitCode: 1,
				},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"NONE"},
					},
				},
			},
		}, nil)

		_, err := tested.(*composeService).isServiceHealthy(ctx, containers, true)
		assert.ErrorContains(t, err, "exited")
	})

	t.Run("healthy container with healthcheck", func(t *testing.T) {
		containerID := "test-container-id"
		containers := Containers{
			{ID: containerID},
		}

		// Container with actual healthcheck that is healthy
		apiClient.EXPECT().ContainerInspect(ctx, containerID, gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:   containerID,
				Name: "test-container",
				State: &container.State{
					Status: "running",
					Health: &container.Health{
						Status: container.Healthy,
					},
				},
				Config: &container.Config{
					Healthcheck: &container.HealthConfig{
						Test: []string{"CMD", "curl", "-f", "http://localhost"},
					},
				},
			},
		}, nil)

		isHealthy, err := tested.(*composeService).isServiceHealthy(ctx, containers, false)
		assert.NilError(t, err)
		assert.Equal(t, true, isHealthy, "Container with healthy status should be healthy")
	})
}

func TestCreateMobyContainer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("").AnyTimes()
	apiClient.EXPECT().ImageInspect(anyCancellableContext(), gomock.Any()).Return(client.ImageInspectResult{}, nil).AnyTimes()

	// force `RuntimeVersion` to fetch fresh version
	runtimeVersion = runtimeVersionCache{}
	apiClient.EXPECT().ServerVersion(gomock.Any(), gomock.Any()).Return(client.ServerVersionResult{
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

	var got client.ContainerCreateOptions
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
		got = opts
		return client.ContainerCreateResult{ID: "an-id"}, nil
	})

	apiClient.EXPECT().ContainerInspect(gomock.Any(), gomock.Eq("an-id"), gomock.Any()).Times(1).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:              "an-id",
			Name:            "a-name",
			Config:          &container.Config{},
			NetworkSettings: &container.NetworkSettings{},
		},
	}, nil)

	_, err = tested.(*composeService).createMobyContainer(t.Context(), &project, service, "test", 0, nil, createOptions{
		Labels: make(types.Labels),
	})
	var falseBool bool
	want := client.ContainerCreateOptions{
		Config: &container.Config{
			AttachStdout: true,
			AttachStderr: true,
			Image:        "bork-test",
			Labels: map[string]string{
				"com.docker.compose.config-hash": "8dbce408396f8986266bc5deba0c09cfebac63c95c2238e405c7bee5f1bd84b8",
				"com.docker.compose.depends_on":  "",
			},
		},
		HostConfig: &container.HostConfig{
			PortBindings: network.PortMap{},
			ExtraHosts:   []string{},
			Tmpfs:        map[string]string{},
			Resources: container.Resources{
				OomKillDisable: &falseBool,
			},
			NetworkMode: "b-moby-name",
		},
		NetworkingConfig: &network.NetworkingConfig{
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
		},
		Name: "test",
	}
	assert.DeepEqual(t, want, got, cmpopts.EquateComparable(netip.Addr{}), cmpopts.EquateEmpty())
	assert.NilError(t, err)
}
