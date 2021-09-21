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
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"gotest.tools/v3/assert"

	moby "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	compose "github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/mocks"
)

func TestPs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	api := mocks.NewMockAPIClient(mockCtrl)
	tested.apiClient = api

	ctx := context.Background()
	args := filters.NewArgs(projectFilter(strings.ToLower(testProject)))
	args.Add("label", "com.docker.compose.oneoff=False")
	listOpts := moby.ContainerListOptions{Filters: args, All: true}
	c1, inspect1 := containerDetails("service1", "123", "running", "healthy", 0)
	c2, inspect2 := containerDetails("service1", "456", "running", "", 0)
	c2.Ports = []moby.Port{{PublicPort: 80, PrivatePort: 90, IP: "localhost"}}
	c3, inspect3 := containerDetails("service2", "789", "exited", "", 130)
	api.EXPECT().ContainerList(ctx, listOpts).Return([]moby.Container{c1, c2, c3}, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "123").Return(inspect1, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "456").Return(inspect2, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "789").Return(inspect3, nil)

	containers, err := tested.Ps(ctx, strings.ToLower(testProject), compose.PsOptions{})

	expected := []compose.ContainerSummary{
		{ID: "123", Name: "123", Project: strings.ToLower(testProject), Service: "service1", State: "running", Health: "healthy", Publishers: nil},
		{ID: "456", Name: "456", Project: strings.ToLower(testProject), Service: "service1", State: "running", Health: "", Publishers: []compose.PortPublisher{{URL: "localhost", TargetPort: 90,
			PublishedPort: 80}}},
		{ID: "789", Name: "789", Project: strings.ToLower(testProject), Service: "service2", State: "exited", Health: "", ExitCode: 130, Publishers: nil},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, containers, expected)
}

func containerDetails(service string, id string, status string, health string, exitCode int) (moby.Container, moby.ContainerJSON) {
	container := moby.Container{
		ID:     id,
		Names:  []string{"/" + id},
		Labels: containerLabels(service, false),
		State:  status,
	}
	inspect := moby.ContainerJSON{ContainerJSONBase: &moby.ContainerJSONBase{State: &moby.ContainerState{Status: status, Health: &moby.Health{Status: health}, ExitCode: exitCode}}}
	return container, inspect
}
