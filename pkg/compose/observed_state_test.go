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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestToObservedContainer(t *testing.T) {
	c := container.Summary{
		ID:    "abc123",
		Names: []string{"/testProject-web-1"},
		State: container.StateRunning,
		Labels: map[string]string{
			api.ServiceLabel:         "web",
			api.ConfigHashLabel:      "sha256:aaa",
			api.ImageDigestLabel:     "sha256:bbb",
			api.ContainerNumberLabel: "1",
			api.ProjectLabel:         "testproject",
		},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*network.EndpointSettings{
				"mynet": {NetworkID: "net123"},
			},
		},
	}

	oc := toObservedContainer(c)

	assert.Equal(t, oc.ID, "abc123")
	assert.Equal(t, oc.Name, "testProject-web-1")
	assert.Equal(t, oc.State, container.StateRunning)
	assert.Equal(t, oc.ConfigHash, "sha256:aaa")
	assert.Equal(t, oc.ImageDigest, "sha256:bbb")
	assert.Equal(t, oc.Number, 1)
	assert.Equal(t, oc.ConnectedNetworks["mynet"], "net123")
	assert.Equal(t, oc.Summary.ID, "abc123")
}

func TestToObservedContainerNoNetworkSettings(t *testing.T) {
	c := container.Summary{
		ID:     "def456",
		Names:  []string{"/testProject-db-1"},
		State:  container.StateExited,
		Labels: map[string]string{},
	}

	oc := toObservedContainer(c)

	assert.Equal(t, oc.ID, "def456")
	assert.Equal(t, oc.Number, 0)
	assert.Equal(t, oc.ConfigHash, "")
	assert.Equal(t, oc.ImageDigest, "")
	assert.Equal(t, len(oc.ConnectedNetworks), 0)
}

func TestCollectObservedState(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	apiClient := mocks.NewMockAPIClient(mockCtrl)
	cli := mocks.NewMockCli(mockCtrl)
	tested, err := NewComposeService(cli)
	assert.NilError(t, err)
	cli.EXPECT().Client().Return(apiClient).AnyTimes()

	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"web": {Name: "web"},
			"db":  {Name: "db"},
		},
		Networks: types.Networks{
			"frontend": {Name: "myproject_frontend"},
		},
		Volumes: types.Volumes{
			"data": {Name: "myproject_data"},
		},
	}

	// Mock ContainerList
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{
		Items: []container.Summary{
			{
				ID:    "c1",
				Names: []string{"/myproject-web-1"},
				State: container.StateRunning,
				Labels: map[string]string{
					api.ServiceLabel:         "web",
					api.ProjectLabel:         "myproject",
					api.ConfigHashLabel:      "hash1",
					api.ContainerNumberLabel: "1",
					api.OneoffLabel:          "False",
				},
			},
			{
				ID:    "c2",
				Names: []string{"/myproject-db-1"},
				State: container.StateRunning,
				Labels: map[string]string{
					api.ServiceLabel:         "db",
					api.ProjectLabel:         "myproject",
					api.ConfigHashLabel:      "hash2",
					api.ContainerNumberLabel: "1",
					api.OneoffLabel:          "False",
				},
			},
			{
				ID:    "c3",
				Names: []string{"/myproject-old-1"},
				State: container.StateExited,
				Labels: map[string]string{
					api.ServiceLabel:         "old",
					api.ProjectLabel:         "myproject",
					api.ConfigHashLabel:      "hash3",
					api.ContainerNumberLabel: "1",
					api.OneoffLabel:          "False",
				},
			},
		},
	}, nil)

	// Mock NetworkList
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{
		Items: []network.Summary{
			{Network: network.Network{
				ID:   "net1",
				Name: "myproject_frontend",
				Labels: map[string]string{
					api.NetworkLabel:    "frontend",
					api.ProjectLabel:    "myproject",
					api.ConfigHashLabel: "nethash1",
				},
			}},
		},
	}, nil)

	// Mock VolumeList
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{
		Items: []volume.Volume{
			{
				Name:   "myproject_data",
				Driver: "local",
				Labels: map[string]string{
					api.VolumeLabel:     "data",
					api.ProjectLabel:    "myproject",
					api.ConfigHashLabel: "volhash1",
				},
			},
		},
	}, nil)

	state, err := tested.(*composeService).collectObservedState(t.Context(), project)
	assert.NilError(t, err)

	// Containers classified by service
	assert.Equal(t, len(state.Containers["web"]), 1)
	assert.Equal(t, state.Containers["web"][0].ID, "c1")
	assert.Equal(t, len(state.Containers["db"]), 1)
	assert.Equal(t, state.Containers["db"][0].ID, "c2")

	// Orphan container (service "old" not in project)
	assert.Equal(t, len(state.Orphans), 1)
	assert.Equal(t, state.Orphans[0].ID, "c3")

	// Networks
	assert.Equal(t, len(state.Networks), 1)
	nw := state.Networks["frontend"]
	assert.Equal(t, nw.ID, "net1")
	assert.Equal(t, nw.Name, "myproject_frontend")
	assert.Equal(t, nw.ConfigHash, "nethash1")

	// Volumes
	assert.Equal(t, len(state.Volumes), 1)
	vol := state.Volumes["data"]
	assert.Equal(t, vol.Name, "myproject_data")
	assert.Equal(t, vol.Driver, "local")
	assert.Equal(t, vol.ConfigHash, "volhash1")
}

type capturingEvents struct {
	noopEventProcessor
	resources []api.Resource
}

func (c *capturingEvents) On(events ...api.Resource) {
	c.resources = append(c.resources, events...)
}

func TestEmitRunningEvents(t *testing.T) {
	runningWeb := ObservedContainer{ID: "c-web", Name: "p-web-1", State: container.StateRunning}
	runningDB := ObservedContainer{ID: "c-db", Name: "p-db-1", State: container.StateRunning}
	runningDisabled := ObservedContainer{ID: "c-misc", Name: "p-misc-1", State: container.StateRunning}
	exitedWeb := ObservedContainer{ID: "c-web-old", Name: "p-web-2", State: container.StateExited}

	t.Run("emits Running for active services not in plan", func(t *testing.T) {
		project := &types.Project{
			Services: types.Services{
				"web": {Name: "web"},
				"db":  {Name: "db"},
			},
		}
		observed := &ObservedState{
			Containers: map[string][]ObservedContainer{
				"web": {runningWeb},
				"db":  {runningDB},
			},
		}
		events := &capturingEvents{}

		emitRunningEvents(project, observed, &Plan{}, events)

		assert.Equal(t, len(events.resources), 2)
		names := map[string]bool{}
		for _, r := range events.resources {
			names[r.ID] = true
			assert.Equal(t, r.Text, api.StatusRunning)
		}
		assert.Assert(t, names["Container p-web-1"])
		assert.Assert(t, names["Container p-db-1"])
	})

	t.Run("skips containers belonging to disabled services", func(t *testing.T) {
		// Reproduces issue 13882: `compose run --no-deps misc` leaves the
		// project with project.Services empty (misc moved to DisabledServices).
		// Running containers for disabled services must not be reported.
		project := &types.Project{
			Services: types.Services{},
			DisabledServices: types.Services{
				"misc": {Name: "misc"},
				"db":   {Name: "db"},
			},
		}
		observed := &ObservedState{
			Containers: map[string][]ObservedContainer{
				"misc": {runningDisabled},
				"db":   {runningDB},
			},
		}
		events := &capturingEvents{}

		emitRunningEvents(project, observed, &Plan{}, events)

		assert.Equal(t, len(events.resources), 0)
	})

	t.Run("skips containers that have a plan operation", func(t *testing.T) {
		project := &types.Project{
			Services: types.Services{"web": {Name: "web"}},
		}
		observed := &ObservedState{
			Containers: map[string][]ObservedContainer{"web": {runningWeb}},
		}
		plan := &Plan{}
		plan.addNode(Operation{
			Type:      OpStopContainer,
			Container: &container.Summary{ID: runningWeb.ID},
		}, "")
		events := &capturingEvents{}

		emitRunningEvents(project, observed, plan, events)

		assert.Equal(t, len(events.resources), 0)
	})

	t.Run("skips non-running containers", func(t *testing.T) {
		project := &types.Project{
			Services: types.Services{"web": {Name: "web"}},
		}
		observed := &ObservedState{
			Containers: map[string][]ObservedContainer{"web": {exitedWeb}},
		}
		events := &capturingEvents{}

		emitRunningEvents(project, observed, &Plan{}, events)

		assert.Equal(t, len(events.resources), 0)
	})
}
