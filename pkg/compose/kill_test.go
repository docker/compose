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
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/types"
	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
)

const testProject = "testProject"

var tested = composeService{}

func TestKillAll(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	project := types.Project{Name: strings.ToLower(testProject), Services: []types.ServiceConfig{testService("service1"), testService("service2")}}

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, projectFilterListOpt()).Return(
		[]moby.Container{testContainer("service1", "123", false), testContainer("service1", "456", false), testContainer("service2", "789", false)}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", "").Return(nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "456", "").Return(nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "789", "").Return(nil)

	err := tested.kill(ctx, &project, compose.KillOptions{})
	assert.NilError(t, err)
}

func TestKillSignal(t *testing.T) {
	const serviceName = "service1"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	project := types.Project{Name: strings.ToLower(testProject), Services: []types.ServiceConfig{testService(serviceName)}}
	listOptions := moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject)),
			serviceFilter(serviceName)),
		All: true,
	}

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, listOptions).Return([]moby.Container{testContainer(serviceName, "123", false)}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", "SIGTERM").Return(nil)

	err := tested.kill(ctx, &project, compose.KillOptions{Services: []string{serviceName}, Signal: "SIGTERM"})
	assert.NilError(t, err)
}

func testService(name string) types.ServiceConfig {
	return types.ServiceConfig{Name: name}
}

func testContainer(service string, id string, oneOff bool) moby.Container {
	return moby.Container{
		ID:     id,
		Names:  []string{id},
		Labels: containerLabels(service, oneOff),
	}
}

func containerLabels(service string, oneOff bool) map[string]string {
	workingdir, _ := filepath.Abs("testdata")
	composefile := filepath.Join(workingdir, "compose.yaml")
	labels := map[string]string{
		compose.ServiceLabel:     service,
		compose.ConfigFilesLabel: composefile,
		compose.WorkingDirLabel:  workingdir,
		compose.ProjectLabel:     strings.ToLower(testProject)}
	if oneOff {
		labels[compose.OneoffLabel] = "True"
	}
	return labels
}

func anyCancellableContext() gomock.Matcher {
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	cancel()
	return gomock.AssignableToTypeOf(ctxWithCancel)
}

func projectFilterListOpt() moby.ContainerListOptions {
	return moby.ContainerListOptions{
		Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject))),
		All:     true,
	}
}
