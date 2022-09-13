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

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/golang/mock/gomock"
	"gotest.tools/assert"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
)

func TestContainerName(t *testing.T) {
	s := types.ServiceConfig{
		Name:          "testservicename",
		ContainerName: "testcontainername",
		Scale:         1,
		Deploy:        &types.DeployConfig{},
	}
	ret, err := getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, s.Scale)

	var zero uint64 // = 0
	s.Deploy.Replicas = &zero
	ret, err = getScale(s)
	assert.NilError(t, err)
	assert.Equal(t, ret, int(*s.Deploy.Replicas))

	var two uint64 = 2
	s.Deploy.Replicas = &two
	_, err = getScale(s)
	assert.Error(t, err, fmt.Sprintf(doubledContainerNameWarning, s.Name, s.ContainerName))
}

func TestServiceLinks(t *testing.T) {
	const dbContainerName = "/" + testProject + "-db-1"
	const webContainerName = "/" + testProject + "-web-1"
	s := types.ServiceConfig{
		Name:  "web",
		Scale: 1,
	}

	containerListOptions := moby.ContainerListOptions{
		Filters: filters.NewArgs(
			projectFilter(testProject),
			serviceFilter("db"),
			oneOffFilter(false),
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
		containerListOptionsOneOff := moby.ContainerListOptions{
			Filters: filters.NewArgs(
				projectFilter(testProject),
				serviceFilter("web"),
				oneOffFilter(false),
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
		dbService := types.ServiceConfig{Name: "db", Scale: 0}
		redisService := types.ServiceConfig{Name: "redis", Scale: 0}
		project := types.Project{Name: strings.ToLower(testProject), Services: []types.ServiceConfig{dbService, redisService}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: ServiceConditionRunningOrHealthy},
			"redis": {Condition: ServiceConditionRunningOrHealthy},
		}
		assert.NilError(t, tested.waitDependencies(context.Background(), &project, dependencies))
	})
	t.Run("should skip dependencies with condition service_started", func(t *testing.T) {
		dbService := types.ServiceConfig{Name: "db", Scale: 1}
		redisService := types.ServiceConfig{Name: "redis", Scale: 1}
		project := types.Project{Name: strings.ToLower(testProject), Services: []types.ServiceConfig{dbService, redisService}}
		dependencies := types.DependsOnConfig{
			"db":    {Condition: types.ServiceConditionStarted},
			"redis": {Condition: types.ServiceConditionStarted},
		}
		assert.NilError(t, tested.waitDependencies(context.Background(), &project, dependencies))
	})
}
