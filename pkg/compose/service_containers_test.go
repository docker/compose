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
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	logrustest "github.com/sirupsen/logrus/hooks/test"
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
			api.ConfigHashLabel,
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
				api.ConfigHashLabel,
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
	t.Run("should skip zero-replica dependencies after service hashing", func(t *testing.T) {
		replicas := 0
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"app": {
				Name: "app", WorkloadSpec: types.WorkloadSpec{DependsOn: types.DependsOnConfig{
					"disabled": {
						Condition: ServiceConditionRunningOrHealthy,
						Required:  true,
					},
				}},
			},
			"disabled": {
				Name:   "disabled",
				Deploy: &types.DeployConfig{Replicas: &replicas},
			},
		}}

		_, err := ServiceHash(project.Services["disabled"])
		assert.NilError(t, err)

		assert.NilError(t, tested.(*composeService).waitDependencies(
			t.Context(), &project, "app", project.Services["app"].DependsOn, nil, 0,
		))
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
	t.Run("missing required dependency is an error", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: true},
		}
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, nil, 0)
		assert.Error(t, err, "app is missing dependency db")
	})
	t.Run("a dependency's pre_start hook runner does not count as the dependency being present", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1), PreStart: []types.PreStartHook{{}}},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: true},
		}
		// Only a retained hook runner exists (e.g. its own success removal is
		// still in flight, or it failed and was kept for inspection) — no
		// real "db" replica container.
		containers := Containers{{
			ID:     "db-hook-runner",
			Names:  []string{"/db-hook-runner"},
			Labels: map[string]string{api.ServiceLabel: "db", api.HookLabel: "pre_start"},
		}}
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers, 0)
		assert.Error(t, err, "app is missing dependency db")
	})
	t.Run("missing optional dependency is only a warning", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: ServiceConditionRunningOrHealthy, Required: false},
		}
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, nil, 0))
	})
	t.Run("failing optional dependency is skipped, not an error", func(t *testing.T) {
		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"db": {Condition: types.ServiceConditionHealthy, Required: false},
		}
		containers := Containers{{
			ID:     "db-ctr",
			Names:  []string{"/db-ctr"},
			Labels: map[string]string{api.ServiceLabel: "db"},
		}}
		// The dependency exited: a required dependency would fail the wait,
		// an optional one is skipped after the first poll.
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).Return(client.ContainerInspectResult{
			Container: container.InspectResponse{
				ID:    "db-ctr",
				Name:  "/db-ctr",
				State: &container.State{Status: container.StateExited, ExitCode: 1},
			},
		}, nil)
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers, 0))
	})
	t.Run("dry-run completes service_completed_successfully immediately", func(t *testing.T) {
		tested.(*composeService).dryRun = true

		project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
			"init": {Name: "init", Scale: intPtr(1)},
		}}
		dependencies := types.DependsOnConfig{
			"init": {Condition: types.ServiceConditionCompletedSuccessfully, Required: true},
		}
		containers := Containers{{
			ID:     "init-ctr",
			Names:  []string{"/init-ctr"},
			Labels: map[string]string{api.ServiceLabel: "init"},
		}}
		// no ContainerInspect expectation: dry-run must not inspect a
		// container that was never created.
		assert.NilError(t, tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers, 0))
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

	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).Return(client.PingResult{
		APIVersion: "1.44",
	}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.44").AnyTimes()

	service := types.ServiceConfig{
		Name: "test", ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{
			"a": {
				Priority: 10,
			},
			"b": {
				Priority: 100,
			},
		}},
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

func TestCreateMobyContainerLegacyAPI(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("").AnyTimes()
	apiClient.EXPECT().ImageInspect(anyCancellableContext(), gomock.Any()).
		Return(client.ImageInspectResult{}, nil).AnyTimes()

	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.43"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.43").AnyTimes()

	service := types.ServiceConfig{
		Name: "test", ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{
			"a": {Priority: 10},
			"b": {Priority: 100},
		}},
	}
	project := types.Project{
		Name: "bork",
		Services: types.Services{
			"test": service,
		},
		Networks: types.Networks{
			"a": types.NetworkConfig{Name: "a-moby-name"},
			"b": types.NetworkConfig{Name: "b-moby-name"},
		},
	}

	var gotCreate client.ContainerCreateOptions
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
			gotCreate = opts
			return client.ContainerCreateResult{ID: "an-id"}, nil
		})

	// For API < 1.44, the secondary network "a" should be connected via NetworkConnect.
	var gotConnect client.NetworkConnectOptions
	connectCall := apiClient.EXPECT().
		NetworkConnect(gomock.Any(), gomock.Eq("a-moby-name"), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.NetworkConnectOptions) (client.NetworkConnectResult, error) {
			gotConnect = opts
			return client.NetworkConnectResult{}, nil
		})

	apiClient.EXPECT().ContainerInspect(gomock.Any(), gomock.Eq("an-id"), gomock.Any()).
		Times(1).After(connectCall).Return(client.ContainerInspectResult{
		Container: container.InspectResponse{
			ID:     "an-id",
			Name:   "a-name",
			Config: &container.Config{},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"b-moby-name": {
						IPAMConfig: &network.EndpointIPAMConfig{},
						Aliases:    []string{"bork-test-0"},
					},
					"a-moby-name": {
						IPAMConfig: &network.EndpointIPAMConfig{},
						Aliases:    []string{"bork-test-0"},
					},
				},
			},
		},
	}, nil)

	_, err = tested.(*composeService).createMobyContainer(t.Context(), &project, service, "test", 0, nil, createOptions{
		Labels:            make(types.Labels),
		UseNetworkAliases: true,
	})
	assert.NilError(t, err)

	// ContainerCreate should only have the primary network (b, highest priority)
	assert.Check(t, gotCreate.NetworkingConfig != nil)
	assert.Equal(t, len(gotCreate.NetworkingConfig.EndpointsConfig), 1)
	_, hasPrimary := gotCreate.NetworkingConfig.EndpointsConfig["b-moby-name"]
	assert.Check(t, hasPrimary, "primary network b-moby-name should be in ContainerCreate EndpointsConfig")

	// NetworkConnect should have been called for the secondary network "a"
	assert.Equal(t, gotConnect.Container, "an-id")
	assert.Check(t, gotConnect.EndpointConfig != nil)
}

func TestCreateMobyContainerLegacyAPI_NetworkConnectFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{}).AnyTimes()
	apiClient.EXPECT().DaemonHost().Return("").AnyTimes()
	apiClient.EXPECT().ImageInspect(anyCancellableContext(), gomock.Any()).
		Return(client.ImageInspectResult{}, nil).AnyTimes()

	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.43"}, nil).AnyTimes()
	apiClient.EXPECT().ClientVersion().Return("1.43").AnyTimes()

	service := types.ServiceConfig{
		Name: "test", ContainerSpec: types.ContainerSpec{Networks: map[string]*types.ServiceNetworkConfig{
			"a": {Priority: 10},
			"b": {Priority: 100},
		}},
	}
	project := types.Project{
		Name: "bork",
		Services: types.Services{
			"test": service,
		},
		Networks: types.Networks{
			"a": types.NetworkConfig{Name: "a-moby-name"},
			"b": types.NetworkConfig{Name: "b-moby-name"},
		},
	}

	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{ID: "an-id"}, nil)

	// NetworkConnect fails
	connectErr := errors.New("network connect failed")
	apiClient.EXPECT().NetworkConnect(gomock.Any(), gomock.Eq("a-moby-name"), gomock.Any()).
		Return(client.NetworkConnectResult{}, connectErr)

	// ContainerRemove should be called to clean up the orphan container
	apiClient.EXPECT().ContainerRemove(gomock.Any(), gomock.Eq("an-id"), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
			assert.Check(t, opts.Force, "ContainerRemove should use Force")
			return client.ContainerRemoveResult{}, nil
		})

	_, err = tested.(*composeService).createMobyContainer(t.Context(), &project, service, "test", 0, nil, createOptions{
		Labels:            make(types.Labels),
		UseNetworkAliases: true,
	})
	assert.ErrorContains(t, err, "network connect failed")
}

func TestRuntimeAPIVersionCachesNegotiation(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested := &composeService{dockerCli: cli}

	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	// Ping reports the server's max API version (1.44), but after negotiation
	// the client may settle on a lower version (1.43) — e.g. when the client
	// SDK caps at an older version. RuntimeAPIVersion must return the negotiated
	// ClientVersion, not the server's raw APIVersion.
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).Return(client.PingResult{
		APIVersion: "1.44",
	}, nil).Times(1)
	apiClient.EXPECT().ClientVersion().Return("1.43").Times(1)

	version, err := tested.RuntimeAPIVersion(t.Context())
	assert.NilError(t, err)
	assert.Equal(t, version, "1.43")

	version, err = tested.RuntimeAPIVersion(t.Context())
	assert.NilError(t, err)
	assert.Equal(t, version, "1.43")
}

func TestRuntimeAPIVersionRetriesOnTransientError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested := &composeService{dockerCli: cli}

	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	// First call: Ping fails with a transient error
	firstCall := apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{}, context.DeadlineExceeded).Times(1)

	// Second call: Ping succeeds after the transient failure
	apiClient.EXPECT().Ping(gomock.Any(), client.PingOptions{NegotiateAPIVersion: true}).
		Return(client.PingResult{APIVersion: "1.44"}, nil).Times(1).After(firstCall)
	apiClient.EXPECT().ClientVersion().Return("1.44").Times(1)

	// First call should return the transient error
	_, err := tested.RuntimeAPIVersion(t.Context())
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Second call should succeed — error was not cached
	version, err := tested.RuntimeAPIVersion(t.Context())
	assert.NilError(t, err)
	assert.Equal(t, version, "1.44")

	// Third call should return the cached value without calling Ping again
	version, err = tested.RuntimeAPIVersion(t.Context())
	assert.NilError(t, err)
	assert.Equal(t, version, "1.44")
}

// A relay stands in for the service on the network but is a shell-less
// scratch binary: post_start has no process inside it to act on. The
// compose spec doesn't forbid declaring hooks on a provider: service, so
// this must be guarded explicitly rather than assumed unreachable. No
// ExecCreate expectation is set: per newStartTestService, gomock fails the
// test if the hook still runs.
func TestStartServiceContainerSkipsHooksForRelay(t *testing.T) {
	svc, apiClient, _ := newStartTestService(t)

	service := types.ServiceConfig{
		Name: "db",
		PostStart: []types.ServiceHook{
			{Command: types.ShellCommand{"echo", "hi"}},
		},
	}
	relay := serviceContainer("db", 1, container.StateCreated)
	relay.Labels[api.RelayLabel] = "abc123"

	apiClient.EXPECT().ContainerStart(gomock.Any(), relay.ID, gomock.Any()).
		Return(client.ContainerStartResult{}, nil)

	err := svc.startServiceContainer(t.Context(), &types.Project{}, service, relay, nil)
	assert.NilError(t, err)
}

// TestWaitDependencyDeadline locks the timeout semantics of the dependency
// wait: an expired deadline surfaces as "timeout waiting for dependencies",
// while a plain user cancellation is not a wait failure.
func TestWaitDependencyDeadline(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
		"db": {Name: "db", Scale: intPtr(1)},
	}}
	dependencies := types.DependsOnConfig{
		"db": {Condition: types.ServiceConditionHealthy, Required: true},
	}
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}

	t.Run("expired deadline is an error", func(t *testing.T) {
		// Timeout shorter than the first 500ms poll tick: the deadline fires
		// before any condition check, and must not be swallowed.
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", dependencies, containers, 50*time.Millisecond)
		assert.Error(t, err, "timeout waiting for dependencies")
	})

	t.Run("user cancellation is not a wait failure", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := tested.(*composeService).waitDependencies(ctx, &project, "app", dependencies, containers, 0)
		assert.NilError(t, err)
	})

	// Regression guard for the runtime fallback warning on waitDependency's
	// default branch — see the comment on that branch for why it must stay.
	t.Run("unsupported condition warns and returns without waiting", func(t *testing.T) {
		hook := logrustest.NewGlobal()
		unsupportedDeps := types.DependsOnConfig{
			"db": {Condition: "some_future_condition", Required: true},
		}
		err := tested.(*composeService).waitDependencies(t.Context(), &project, "app", unsupportedDeps, containers, 2*time.Second)
		assert.NilError(t, err)

		var messages []string
		for _, e := range hook.AllEntries() {
			messages = append(messages, e.Message)
		}
		joined := strings.Join(messages, "\n")
		assert.Assert(t, strings.Contains(joined, `service "app": unsupported depends_on condition "some_future_condition"`), joined)
	})
}

// TestOptionalDependencyInspectFailure locks what an optional (required:
// false) dependency's failing inspection means. A genuine inspection error
// (the daemon doesn't know the container, the API call fails) says the
// dependency didn't start, so it is skipped. A failure caused by the wait's
// context being done says nothing about the dependency: it must not be
// turned into a skipped dependency, i.e. into a successful wait, and the end
// of the wait is left to waitDependency (a deadline is an error, a plain
// cancellation stays silent). A required dependency is never skipped.
func TestOptionalDependencyInspectFailure(t *testing.T) {
	const skipped = "Skipped: "
	conditions := []string{ServiceConditionRunningOrHealthy, types.ServiceConditionHealthy}
	dbContainers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	project := types.Project{Name: strings.ToLower(testProject), Services: types.Services{
		"db": {Name: "db", Scale: intPtr(1)},
	}}

	newTested := func(t *testing.T) (*composeService, *mocks.MockAPIClient, *capturingEvents) {
		t.Helper()
		mockCtrl := gomock.NewController(t)
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		events := &capturingEvents{}
		tested, err := NewComposeService(cli, WithEventProcessor(events))
		assert.NilError(t, err)
		return tested.(*composeService), apiClient, events
	}
	wasSkipped := func(events *capturingEvents) bool {
		for _, r := range events.resources {
			if strings.HasPrefix(r.Text, skipped) {
				return true
			}
		}
		return false
	}
	dependenciesOn := func(condition string, required bool) types.DependsOnConfig {
		return types.DependsOnConfig{"db": {Condition: condition, Required: required}}
	}
	// inspectFailsWithContext makes ContainerInspect fail the way the client
	// does when the context it was given is done: it waits for it and
	// returns its error, wrapped. It must be reached at least once (or the
	// test would pass without exercising the inspection); once the context is
	// done, a pending poll tick may win the select against ctx.Done() for one
	// more check, so there is no upper bound.
	inspectFailsWithContext := func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				<-ctx.Done()
				return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", ctx.Err())
			}).MinTimes(1)
	}

	for _, condition := range conditions {
		t.Run(condition, func(t *testing.T) {
			t.Run("a genuine inspection error skips an optional dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, errors.New("Error response from daemon: No such container: db-ctr"))
				err := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, false), dbContainers, 0)
				assert.NilError(t, err)
				assert.Assert(t, wasSkipped(events))
			})

			t.Run("a genuine inspection error fails a required dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, errors.New("Error response from daemon: No such container: db-ctr"))
				err := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, true), dbContainers, 0)
				assert.ErrorContains(t, err, "No such container: db-ctr")
				assert.Assert(t, !wasSkipped(events))
			})

			t.Run("a cancellation during the inspection is not a skipped optional dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, events := newTested(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
						cancel()
						return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", context.Canceled)
					}).MinTimes(1) // a pending poll tick may win the select once more
				// a plain cancellation is not a wait failure (see
				// TestWaitDependencyDeadline), but it is no skip either
				err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, false), dbContainers, 0)
				assert.NilError(t, err)
				assert.Assert(t, !wasSkipped(events))
			})

			t.Run("a cancellation during the inspection still fails a required dependency", func(t *testing.T) {
				t.Parallel()
				tested, apiClient, _ := newTested(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					DoAndReturn(func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
						cancel()
						return client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", context.Canceled)
					})
				err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, true), dbContainers, 0)
				assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
			})

			for _, required := range []bool{false, true} {
				t.Run(fmt.Sprintf("a deadline inherited from the caller expiring during the inspection is an error (required=%t)", required), func(t *testing.T) {
					t.Parallel()
					tested, apiClient, events := newTested(t)
					inspectFailsWithContext(apiClient)
					// the first poll (500ms) starts the inspection, which only
					// ends with the deadline
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					err := tested.waitDependencies(ctx, &project, "app", dependenciesOn(condition, required), dbContainers, 0)
					assert.Error(t, err, "timeout waiting for dependencies")
					assert.Assert(t, !wasSkipped(events))
				})
			}

			// The wait's own timeout expiring during the inspection must end
			// the wait exactly as it does when it expires between two polls:
			// whatever waitDependency decides for an optional dependency, the
			// check functions must not decide differently.
			for _, required := range []bool{false, true} {
				t.Run(fmt.Sprintf("the wait's own timeout expiring during the inspection ends like it does between polls (required=%t)", required), func(t *testing.T) {
					t.Parallel()
					tested, apiClient, _ := newTested(t)
					inspectFailsWithContext(apiClient)
					duringInspect := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, required), dbContainers, time.Second)

					// shorter than the first poll: expires before any check
					betweenPolls := tested.waitDependencies(t.Context(), &project, "app", dependenciesOn(condition, required), dbContainers, 50*time.Millisecond)

					assert.Equal(t, fmt.Sprint(duringInspect), fmt.Sprint(betweenPolls))
					if required {
						assert.Error(t, duringInspect, "timeout waiting for dependencies")
					}
				})
			}
		})
	}
}

// TestCheckDependencyDoneContext tests the check functions directly with a
// context that is already done, so no poll timing is involved: an optional
// dependency is "not done yet" (the wait's loop then sees ctx.Done()), never
// skipped; a required one keeps reporting the failure.
func TestCheckDependencyDoneContext(t *testing.T) {
	containers := Containers{{
		ID:     "db-ctr",
		Names:  []string{"/db-ctr"},
		Labels: map[string]string{api.ServiceLabel: "db"},
	}}
	expired := func(t *testing.T) context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	canceled := func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	contexts := map[string]func(*testing.T) context.Context{"canceled": canceled, "expired": expired}
	checks := map[string]func(*composeService, context.Context, types.ServiceDependency) (bool, error){
		"running_or_healthy": func(s *composeService, ctx context.Context, c types.ServiceDependency) (bool, error) {
			return s.checkDependencyRunningOrHealthy(ctx, "db", c, containers)
		},
		"service_healthy": func(s *composeService, ctx context.Context, c types.ServiceDependency) (bool, error) {
			return s.checkDependencyHealthy(ctx, "db", c, containers)
		},
	}

	for checkName, check := range checks {
		for ctxName, newCtx := range contexts {
			t.Run(checkName+"/"+ctxName, func(t *testing.T) {
				t.Parallel()
				mockCtrl := gomock.NewController(t)
				apiClient := mocks.NewMockAPIClient(mockCtrl)
				cli := mocks.NewMockCli(mockCtrl)
				cli.EXPECT().Client().Return(apiClient).AnyTimes()
				events := &capturingEvents{}
				svc, err := NewComposeService(cli, WithEventProcessor(events))
				assert.NilError(t, err)
				tested := svc.(*composeService)

				ctx := newCtx(t)
				apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
					Return(client.ContainerInspectResult{}, fmt.Errorf("inspect db-ctr: %w", ctx.Err())).Times(2)

				done, err := check(tested, ctx, types.ServiceDependency{Required: false})
				assert.NilError(t, err)
				assert.Assert(t, !done, "an optional dependency must not complete on a done context")
				assert.Equal(t, len(events.resources), 0, "no skipped event expected, got %v", events.resources)

				_, err = check(tested, ctx, types.ServiceDependency{Required: true})
				assert.Assert(t, errors.Is(err, ctx.Err()), "got %v", err)
			})
		}
	}

	// service_completed_successfully never skipped an optional dependency on
	// an inspection error (only on a non-zero exit code): unchanged.
	t.Run("service_completed_successfully", func(t *testing.T) {
		t.Parallel()
		mockCtrl := gomock.NewController(t)
		apiClient := mocks.NewMockAPIClient(mockCtrl)
		cli := mocks.NewMockCli(mockCtrl)
		cli.EXPECT().Client().Return(apiClient).AnyTimes()
		svc, err := NewComposeService(cli)
		assert.NilError(t, err)

		ctx := canceled(t)
		apiClient.EXPECT().ContainerInspect(gomock.Any(), "db-ctr", gomock.Any()).
			Return(client.ContainerInspectResult{}, ctx.Err())
		done, err := svc.(*composeService).checkDependencyCompleted(ctx, "db", types.ServiceDependency{Required: false}, containers)
		assert.Assert(t, errors.Is(err, context.Canceled), "got %v", err)
		assert.Assert(t, !done)
	})
}
