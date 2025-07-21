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

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

const testProject = "testProject"

func TestKillAll(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)

	api.EXPECT().ContainerList(t.Context(), client.ContainerListOptions{
		Filters: projectFilter(name).Add("label", hasConfigHashLabel()),
	}).Return(client.ContainerListResult{
		Items: []container.Summary{
			testContainer("service1", "123", false),
			testContainer("service1", "456", false),
			testContainer("service2", "789", false),
		},
	}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{
			Items: []network.Summary{{
				Network: network.Network{ID: "abc123", Name: "testProject_default"},
			}},
		}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", client.ContainerKillOptions{}).Return(client.ContainerKillResult{}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "456", client.ContainerKillOptions{}).Return(client.ContainerKillResult{}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "789", client.ContainerKillOptions{}).Return(client.ContainerKillResult{}, nil)

	err = tested.Kill(t.Context(), name, compose.KillOptions{})
	assert.NilError(t, err)
}

func TestKillSignal(t *testing.T) {
	const serviceName = "service1"
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	name := strings.ToLower(testProject)
	listOptions := client.ContainerListOptions{
		Filters: projectFilter(name).Add("label", serviceFilter(serviceName), hasConfigHashLabel()),
	}

	api.EXPECT().ContainerList(t.Context(), listOptions).Return(client.ContainerListResult{
		Items: []container.Summary{testContainer(serviceName, "123", false)},
	}, nil)
	api.EXPECT().VolumeList(
		gomock.Any(),
		client.VolumeListOptions{
			Filters: projectFilter(strings.ToLower(testProject)),
		}).
		Return(client.VolumeListResult{}, nil)
	api.EXPECT().NetworkList(gomock.Any(), client.NetworkListOptions{Filters: projectFilter(strings.ToLower(testProject))}).
		Return(client.NetworkListResult{
			Items: []network.Summary{{
				Network: network.Network{ID: "abc123", Name: "testProject_default"},
			}},
		}, nil)
	api.EXPECT().ContainerKill(anyCancellableContext(), "123", client.ContainerKillOptions{
		Signal: "SIGTERM",
	}).Return(client.ContainerKillResult{}, nil)

	err = tested.Kill(t.Context(), name, compose.KillOptions{Services: []string{serviceName}, Signal: "SIGTERM"})
	assert.NilError(t, err)
}

func testContainer(service string, id string, oneOff bool) container.Summary {
	// canonical docker names in the API start with a leading slash, some
	// parts of Compose code will attempt to strip this off, so make sure
	// it's consistently present
	name := "/" + strings.TrimPrefix(id, "/")
	return container.Summary{
		ID:     id,
		Names:  []string{name},
		Labels: containerLabels(service, oneOff),
		State:  container.StateExited,
	}
}

func containerLabels(service string, oneOff bool) map[string]string {
	workingdir := "/src/pkg/compose/testdata"
	composefile := filepath.Join(workingdir, "compose.yaml")
	labels := map[string]string{
		compose.ServiceLabel:     service,
		compose.ConfigFilesLabel: composefile,
		compose.WorkingDirLabel:  workingdir,
		compose.ProjectLabel:     strings.ToLower(testProject),
	}
	if oneOff {
		labels[compose.OneoffLabel] = "True"
	}
	return labels
}

func anyCancellableContext() gomock.Matcher {
	//nolint:forbidigo // This creates a context type for gomock matching, not for actual test usage
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	cancel()
	return gomock.AssignableToTypeOf(ctxWithCancel)
}

func projectFilterListOpt(withOneOff bool) client.ContainerListOptions {
	filter := projectFilter(strings.ToLower(testProject)).Add("label", hasConfigHashLabel())
	if !withOneOff {
		filter.Add("label", oneOffFilter(false))
	}
	return client.ContainerListOptions{
		Filters: filter,
		All:     true,
	}
}
