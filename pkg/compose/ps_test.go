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
	"net/netip"
	"strings"
	"testing"

	containerType "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

func TestPs(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	api, cli := prepareMocks(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)

	listOpts := client.ContainerListOptions{
		Filters: projectFilter(strings.ToLower(testProject)).Add("label", hasConfigHashLabel(), oneOffFilter(false)),
		All:     false,
	}
	c1, inspect1 := containerDetails("service1", "123", containerType.StateRunning, containerType.Healthy, 0)
	c2, inspect2 := containerDetails("service1", "456", containerType.StateRunning, "", 0)
	c2.Ports = []containerType.PortSummary{{PublicPort: 80, PrivatePort: 90, IP: netip.MustParseAddr("127.0.0.1")}}
	c3, inspect3 := containerDetails("service2", "789", containerType.StateExited, "", 130)
	api.EXPECT().ContainerList(t.Context(), listOpts).Return(client.ContainerListResult{
		Items: []containerType.Summary{c1, c2, c3},
	}, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "123", gomock.Any()).Return(client.ContainerInspectResult{Container: inspect1}, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "456", gomock.Any()).Return(client.ContainerInspectResult{Container: inspect2}, nil)
	api.EXPECT().ContainerInspect(anyCancellableContext(), "789", gomock.Any()).Return(client.ContainerInspectResult{Container: inspect3}, nil)

	containers, err := tested.Ps(t.Context(), strings.ToLower(testProject), compose.PsOptions{})

	expected := []compose.ContainerSummary{
		{
			ID: "123", Name: "123", Names: []string{"/123"}, Image: "foo", Project: strings.ToLower(testProject), Service: "service1",
			State:      containerType.StateRunning,
			Health:     containerType.Healthy,
			Publishers: []compose.PortPublisher{},
			Labels: map[string]string{
				compose.ProjectLabel:     strings.ToLower(testProject),
				compose.ConfigFilesLabel: "/src/pkg/compose/testdata/compose.yaml",
				compose.WorkingDirLabel:  "/src/pkg/compose/testdata",
				compose.ServiceLabel:     "service1",
			},
		},
		{
			ID: "456", Name: "456", Names: []string{"/456"}, Image: "foo", Project: strings.ToLower(testProject), Service: "service1",
			State:      containerType.StateRunning,
			Publishers: []compose.PortPublisher{{URL: "127.0.0.1", TargetPort: 90, PublishedPort: 80}},
			Labels: map[string]string{
				compose.ProjectLabel:     strings.ToLower(testProject),
				compose.ConfigFilesLabel: "/src/pkg/compose/testdata/compose.yaml",
				compose.WorkingDirLabel:  "/src/pkg/compose/testdata",
				compose.ServiceLabel:     "service1",
			},
		},
		{
			ID: "789", Name: "789", Names: []string{"/789"}, Image: "foo", Project: strings.ToLower(testProject), Service: "service2",
			State:      containerType.StateExited,
			ExitCode:   130,
			Publishers: []compose.PortPublisher{},
			Labels: map[string]string{
				compose.ProjectLabel:     strings.ToLower(testProject),
				compose.ConfigFilesLabel: "/src/pkg/compose/testdata/compose.yaml",
				compose.WorkingDirLabel:  "/src/pkg/compose/testdata",
				compose.ServiceLabel:     "service2",
			},
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, containers, expected)
}

func containerDetails(service string, id string, status containerType.ContainerState, health containerType.HealthStatus, exitCode int) (containerType.Summary, containerType.InspectResponse) {
	ctr := containerType.Summary{
		ID:     id,
		Names:  []string{"/" + id},
		Image:  "foo",
		Labels: containerLabels(service, false),
		State:  status,
	}
	inspect := containerType.InspectResponse{
		State: &containerType.State{
			Status:   status,
			Health:   &containerType.Health{Status: health},
			ExitCode: exitCode,
		},
	}
	return ctr, inspect
}
