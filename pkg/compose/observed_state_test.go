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
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
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

// collectByNameDiscovery mocks empty container/network/volume lists so that only
// the legacy by-name network/volume discovery is exercised.
func collectByNameDiscovery(t *testing.T, project *types.Project, inspect func(apiClient *mocks.MockAPIClient)) (*ObservedState, error) {
	t.Helper()
	svc, apiClient := newTestService(t)
	apiClient.EXPECT().ContainerList(gomock.Any(), gomock.Any()).Return(client.ContainerListResult{}, nil)
	apiClient.EXPECT().NetworkList(gomock.Any(), gomock.Any()).Return(client.NetworkListResult{}, nil)
	apiClient.EXPECT().VolumeList(gomock.Any(), gomock.Any()).Return(client.VolumeListResult{}, nil)
	inspect(apiClient)
	return svc.collectObservedState(t.Context(), project)
}

// TestCollectObservedState_LegacyNetworkMatchedByName verifies that a network
// matching a declared network by name but carrying no compose label is recorded
// as an unmanaged match with an empty ConfigHash, so the reconciler reuses it.
func TestCollectObservedState_LegacyNetworkMatchedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Networks: types.Networks{"frontend": {Name: "myproject_frontend"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().NetworkInspect(gomock.Any(), "myproject_frontend", gomock.Any()).Return(client.NetworkInspectResult{
			Network: network.Inspect{Network: network.Network{ID: "net1", Name: "myproject_frontend"}},
		}, nil)
	})
	assert.NilError(t, err)
	obs, ok := state.Networks["frontend"]
	assert.Assert(t, ok, "legacy network must be discovered by name")
	assert.Equal(t, obs.ID, "net1")
	assert.Equal(t, obs.Name, "myproject_frontend")
	assert.Equal(t, obs.ProjectName, "")
	assert.Equal(t, obs.ConfigHash, "", "unmanaged match must have an empty config hash")
}

// TestCollectObservedState_OwnedNetworkMissingKeyLabelKeepsHash verifies that a
// network owned by this project but missing the network-key label (e.g. written
// by an older Compose) keeps its config-hash, so genuine divergence is still
// detected rather than silently skipped.
func TestCollectObservedState_OwnedNetworkMissingKeyLabelKeepsHash(t *testing.T) {
	project := &types.Project{Name: "myproject", Networks: types.Networks{"frontend": {Name: "myproject_frontend"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().NetworkInspect(gomock.Any(), "myproject_frontend", gomock.Any()).Return(client.NetworkInspectResult{
			Network: network.Inspect{Network: network.Network{ID: "net1", Name: "myproject_frontend", Labels: map[string]string{
				api.ProjectLabel:    "myproject", // owned, but no NetworkLabel
				api.ConfigHashLabel: "realhash",
			}}},
		}, nil)
	})
	assert.NilError(t, err)
	assert.Equal(t, state.Networks["frontend"].ConfigHash, "realhash")
}

// TestCollectObservedState_OwnedVolumeMissingKeyLabelKeepsHash is the volume
// counterpart of the network case above.
func TestCollectObservedState_OwnedVolumeMissingKeyLabelKeepsHash(t *testing.T) {
	project := &types.Project{Name: "myproject", Volumes: types.Volumes{"data": {Name: "myproject_data"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().VolumeInspect(gomock.Any(), "myproject_data", gomock.Any()).Return(client.VolumeInspectResult{
			Volume: volume.Volume{Name: "myproject_data", Labels: map[string]string{
				api.ProjectLabel:    "myproject", // owned, but no VolumeLabel
				api.ConfigHashLabel: "realhash",
			}},
		}, nil)
	})
	assert.NilError(t, err)
	assert.Equal(t, state.Volumes["data"].ConfigHash, "realhash")
}

// TestCollectObservedState_ForeignProjectNetworkMatchedByName verifies that a
// network owned by another project but matching the declared name is recorded
// with an empty ConfigHash (reused untouched) and keeps the foreign project name.
func TestCollectObservedState_ForeignProjectNetworkMatchedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Networks: types.Networks{"frontend": {Name: "shared_net"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().NetworkInspect(gomock.Any(), "shared_net", gomock.Any()).Return(client.NetworkInspectResult{
			Network: network.Inspect{Network: network.Network{ID: "net9", Name: "shared_net", Labels: map[string]string{
				api.ProjectLabel:    "otherproject",
				api.ConfigHashLabel: "foreignhash",
			}}},
		}, nil)
	})
	assert.NilError(t, err)
	obs := state.Networks["frontend"]
	assert.Equal(t, obs.ProjectName, "otherproject")
	assert.Equal(t, obs.ConfigHash, "", "foreign network must not be treated as diverged")
}

// TestCollectObservedState_NetworkNotFoundByName verifies a declared network with
// no live counterpart is left absent so the reconciler schedules a create.
func TestCollectObservedState_NetworkNotFoundByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Networks: types.Networks{"frontend": {Name: "myproject_frontend"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().NetworkInspect(gomock.Any(), "myproject_frontend", gomock.Any()).Return(client.NetworkInspectResult{}, notFoundError{})
	})
	assert.NilError(t, err)
	_, ok := state.Networks["frontend"]
	assert.Assert(t, !ok, "absent network must not be in observed state")
}

// TestCollectObservedState_ExternalNetworkNotInspectedByName verifies external
// networks are excluded from the legacy by-name discovery (no NetworkInspect).
func TestCollectObservedState_ExternalNetworkNotInspectedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Networks: types.Networks{"frontend": {Name: "ext_net", External: true}}}
	state, err := collectByNameDiscovery(t, project, func(_ *mocks.MockAPIClient) {})
	assert.NilError(t, err)
	_, ok := state.Networks["frontend"]
	assert.Assert(t, !ok)
}

// TestWarnUnmanagedNetworks verifies the legacy ownership warnings for networks
// reused by name, and silence for managed/external/absent networks.
func TestWarnUnmanagedNetworks(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Networks: types.Networks{
			"managed":  {Name: "myproject_managed"},
			"unlabel":  {Name: "unlabel_net"},
			"foreign":  {Name: "foreign_net"},
			"external": {Name: "ext_net", External: true},
			"tocreate": {Name: "myproject_tocreate"},
		},
	}
	observed := &ObservedState{
		Networks: map[string]ObservedNetwork{
			"managed":  {Name: "myproject_managed", ProjectName: "myproject", ConfigHash: "h"},
			"unlabel":  {Name: "unlabel_net", ProjectName: ""},
			"foreign":  {Name: "foreign_net", ProjectName: "otherproject"},
			"external": {Name: "ext_net"},
		},
	}

	hook := logrustest.NewGlobal()
	warnUnmanagedNetworks(project, observed)

	var msgs []string
	for _, e := range hook.AllEntries() {
		assert.Equal(t, e.Level, logrus.WarnLevel)
		msgs = append(msgs, e.Message)
	}
	assert.Equal(t, len(msgs), 2, "expected exactly two warnings, got: %v", msgs)
	joined := strings.Join(msgs, "\n")
	assert.Assert(t, strings.Contains(joined, "a network with name unlabel_net exists but was not created by compose"), joined)
	assert.Assert(t, strings.Contains(joined, `a network with name foreign_net exists but was not created for project "myproject"`), joined)
}

// TestCollectObservedState_LegacyVolumeMatchedByName verifies that a volume that
// matches a declared volume by name but carries no compose label (pre-label
// Compose or manually created) is recorded as an unmanaged match with an empty
// ConfigHash, so the reconciler reuses it untouched.
func TestCollectObservedState_LegacyVolumeMatchedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Volumes: types.Volumes{"data": {Name: "myproject_data"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().VolumeInspect(gomock.Any(), "myproject_data", gomock.Any()).Return(client.VolumeInspectResult{
			Volume: volume.Volume{Name: "myproject_data", Driver: "local"},
		}, nil)
	})
	assert.NilError(t, err)
	obs, ok := state.Volumes["data"]
	assert.Assert(t, ok, "legacy volume must be discovered by name")
	assert.Equal(t, obs.Name, "myproject_data")
	assert.Equal(t, obs.ProjectName, "")
	assert.Equal(t, obs.ConfigHash, "", "unmanaged match must have an empty config hash")
}

// TestCollectObservedState_ForeignProjectVolumeMatchedByName verifies that a
// volume owned by another project but matching the declared name is recorded
// with an empty ConfigHash (reused untouched, never recreated) and keeps the
// foreign project name for the warning.
func TestCollectObservedState_ForeignProjectVolumeMatchedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Volumes: types.Volumes{"data": {Name: "shared_data"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().VolumeInspect(gomock.Any(), "shared_data", gomock.Any()).Return(client.VolumeInspectResult{
			Volume: volume.Volume{Name: "shared_data", Driver: "local", Labels: map[string]string{
				api.ProjectLabel:    "otherproject",
				api.ConfigHashLabel: "foreignhash",
			}},
		}, nil)
	})
	assert.NilError(t, err)
	obs := state.Volumes["data"]
	assert.Equal(t, obs.ProjectName, "otherproject")
	assert.Equal(t, obs.ConfigHash, "", "foreign volume must not be treated as diverged")
}

// TestCollectObservedState_VolumeNotFoundByName verifies that a declared volume
// with no live counterpart is left absent so the reconciler schedules a create.
func TestCollectObservedState_VolumeNotFoundByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Volumes: types.Volumes{"data": {Name: "myproject_data"}}}
	state, err := collectByNameDiscovery(t, project, func(apiClient *mocks.MockAPIClient) {
		apiClient.EXPECT().VolumeInspect(gomock.Any(), "myproject_data", gomock.Any()).Return(client.VolumeInspectResult{}, notFoundError{})
	})
	assert.NilError(t, err)
	_, ok := state.Volumes["data"]
	assert.Assert(t, !ok, "absent volume must not be in observed state")
}

// TestCollectObservedState_ExternalVolumeNotInspectedByName verifies external
// volumes are not part of the legacy by-name discovery (no VolumeInspect call:
// gomock would fail on an unexpected call).
func TestCollectObservedState_ExternalVolumeNotInspectedByName(t *testing.T) {
	project := &types.Project{Name: "myproject", Volumes: types.Volumes{"data": {Name: "ext_data", External: true}}}
	state, err := collectByNameDiscovery(t, project, func(_ *mocks.MockAPIClient) {})
	assert.NilError(t, err)
	_, ok := state.Volumes["data"]
	assert.Assert(t, !ok)
}

// TestWarnUnmanagedVolumes verifies the legacy ownership warnings are preserved
// for volumes reused by name, and not emitted for managed or external volumes.
func TestWarnUnmanagedVolumes(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Volumes: types.Volumes{
			"managed":  {Name: "myproject_managed"},
			"unlabel":  {Name: "unlabel_data"},
			"foreign":  {Name: "foreign_data"},
			"external": {Name: "ext_data", External: true},
			"tocreate": {Name: "myproject_tocreate"},
		},
	}
	observed := &ObservedState{
		Volumes: map[string]ObservedVolume{
			"managed":  {Name: "myproject_managed", ProjectName: "myproject", ConfigHash: "h"},
			"unlabel":  {Name: "unlabel_data", ProjectName: ""},
			"foreign":  {Name: "foreign_data", ProjectName: "otherproject"},
			"external": {Name: "ext_data"},
			// "tocreate" absent: will be created, no warning.
		},
	}

	hook := logrustest.NewGlobal()
	warnUnmanagedVolumes(project, observed)

	var msgs []string
	for _, e := range hook.AllEntries() {
		assert.Equal(t, e.Level, logrus.WarnLevel)
		msgs = append(msgs, e.Message)
	}
	assert.Equal(t, len(msgs), 2, "expected exactly two warnings, got: %v", msgs)
	joined := strings.Join(msgs, "\n")
	assert.Assert(t, strings.Contains(joined, `volume "unlabel_data" already exists but was not created by Docker Compose`), joined)
	assert.Assert(t, strings.Contains(joined, `volume "foreign_data" already exists but was created for project "otherproject"`), joined)
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
