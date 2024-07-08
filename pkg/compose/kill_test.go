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
	"path/filepath"
	"strings"
	"testing"

	moby "github.com/docker/docker/api/types"
	containerType "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v2/pkg/api"
)

const testProject = "testProject"

func TestKillAll(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}

	name := strings.ToLower(testProject)

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, containerType.ListOptions{
		Filters: filters.NewArgs(projectFilter(name), hasConfigHashLabel()),
	}).Return(
		[]moby.Container{testContainer("service1", "123", false), testContainer("service1", "456", false), testContainer("service2", "789", false)}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		volume.ListOptions{
			Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject))),
		}).
		Return(volume.ListResponse{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), network.ListOptions{Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject)))}).
		Return([]network.Summary{
			{ID: "abc123", Name: "testProject_default"},
		}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", "").Return(nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "456", "").Return(nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "789", "").Return(nil)

	err := tested.kill(ctx, name, compose.KillOptions{})
	assert.NilError(t, err)
}

func TestKillSignal(t *testing.T) {
	const serviceName = "service1"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested := composeService{
		dockerCli: cli,
	}

	name := strings.ToLower(testProject)
	listOptions := containerType.ListOptions{
		Filters: filters.NewArgs(projectFilter(name), serviceFilter(serviceName), hasConfigHashLabel()),
	}

	ctx := context.Background()
	api.EXPECT().ContainerList(ctx, listOptions).Return([]moby.Container{testContainer(serviceName, "123", false)}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		volume.ListOptions{
			Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject))),
		}).
		Return(volume.ListResponse{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), network.ListOptions{Filters: filters.NewArgs(projectFilter(strings.ToLower(testProject)))}).
		Return([]network.Summary{
			{ID: "abc123", Name: "testProject_default"},
		}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", "SIGTERM").Return(nil)

	err := tested.kill(ctx, name, compose.KillOptions{Services: []string{serviceName}, Signal: "SIGTERM"})
	assert.NilError(t, err)
}

func testContainer(service string, id string, oneOff bool) moby.Container {
	// canonical docker names in the API start with a leading slash, some
	// parts of Compose code will attempt to strip this off, so make sure
	// it's consistently present
	name := "/" + strings.TrimPrefix(id, "/")
	return moby.Container{
		ID:     id,
		Names:  []string{name},
		Labels: containerLabels(service, oneOff),
		State:  ContainerExited,
	}
}

func containerLabels(service string, oneOff bool) map[string]string {
	workingdir := "/src/pkg/compose/testdata"
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

func projectFilterListOpt(withOneOff bool) containerType.ListOptions {
	filter := filters.NewArgs(
		projectFilter(strings.ToLower(testProject)),
		hasConfigHashLabel(),
	)
	if !withOneOff {
		filter.Add("label", fmt.Sprintf("%s=False", compose.OneoffLabel))
	}
	return containerType.ListOptions{
		Filters: filter,
		All:     true,
	}
}
