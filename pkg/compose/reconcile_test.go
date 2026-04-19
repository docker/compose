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
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// noPrompt is a Prompt that should never be called in tests that don't expect it.
func noPrompt(msg string, def bool) (bool, error) {
	panic("unexpected prompt call: " + msg)
}

func alwaysYesPrompt(string, bool) (bool, error) { return true, nil }
func alwaysNoPrompt(string, bool) (bool, error)  { return false, nil }

func defaultReconcileOptions() ReconcileOptions {
	return ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		Inherit:              true,
	}
}

// --- Network tests ---

func TestReconcileNetworks_CreateMissing(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Networks: types.Networks{
			"frontend": {Name: "myproject_frontend"},
			"backend":  {Name: "myproject_backend"},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "CreateNetwork, not found"))
	assert.Equal(t, len(plan.Nodes), 2)
}

func TestReconcileNetworks_ExistingMatch(t *testing.T) {
	nw := types.NetworkConfig{Name: "myproject_frontend"}
	hash, err := NetworkHash(&nw)
	assert.NilError(t, err)

	project := &types.Project{
		Name:     "myproject",
		Networks: types.Networks{"frontend": nw},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net1", Name: "myproject_frontend", ConfigHash: hash},
		},
		Volumes: map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileNetworks_ExternalSkipped(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Networks: types.Networks{
			"ext": {Name: "external_net", External: true},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileNetworks_Diverged(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Networks: types.Networks{
			"frontend": {Name: "myproject_frontend", Driver: "overlay"},
		},
		Services: types.Services{
			"web": {
				Name:     "web",
				Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {
				{
					ID: "c1", Number: 1, State: container.StateRunning,
					Summary: container.Summary{
						ID: "c1",
						Labels: map[string]string{
							api.ServiceLabel:         "web",
							api.ContainerNumberLabel: "1",
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"},
		},
		Volumes: map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	expected := "[] -> #1 service:web:1, StopContainer, network frontend config changed\n" +
		"[1] -> #2 service:web:1, DisconnectNetwork, network frontend recreate\n" +
		"[2] -> #3 network:frontend, RemoveNetwork, config hash diverged\n" +
		"[3] -> #4 network:frontend, CreateNetwork, recreate after config change\n"
	assert.Equal(t, plan.String(), expected)
}

func TestReconcileNetworks_DivergedMultipleServices(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Networks: types.Networks{
			"frontend": {Name: "myproject_frontend", Driver: "overlay"},
		},
		Services: types.Services{
			"web": {
				Name:     "web",
				Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}},
			},
			"api": {
				Name:     "api",
				Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c1", Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1"}},
			}},
			"api": {{
				ID: "c2", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c2", Labels: map[string]string{api.ServiceLabel: "api", api.ContainerNumberLabel: "1"}},
			}},
		},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"},
		},
		Volumes: map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	// Both containers stopped and disconnected before network removal
	assert.Assert(t, containsLine(s, "StopContainer, network frontend config changed"))
	assert.Assert(t, containsLine(s, "DisconnectNetwork, network frontend recreate"))
	assert.Assert(t, containsLine(s, "RemoveNetwork, config hash diverged"))
	assert.Assert(t, containsLine(s, "CreateNetwork, recreate after config change"))
	// 2 stops + 2 disconnects + 1 remove + 1 create = 6 nodes
	assert.Equal(t, len(plan.Nodes), 6)
}

// --- Volume tests ---

func TestReconcileVolumes_CreateMissing(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data"}},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	expected := "[] -> #1 volume:data, CreateVolume, not found\n"
	assert.Equal(t, plan.String(), expected)
}

func TestReconcileVolumes_ExistingMatch(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	hash, err := VolumeHash(vol)
	assert.NilError(t, err)

	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": vol},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "myproject_data", ConfigHash: hash},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileVolumes_ExternalSkipped(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"ext": {Name: "external_vol", External: true}},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileVolumes_DivergedConfirmed(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data", Driver: "local"}},
		Services: types.Services{
			"db": {
				Name: "db",
				Volumes: []types.ServiceVolumeConfig{
					{Source: "data", Type: "volume"},
				},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db": {{
				ID: "c1", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db", api.ContainerNumberLabel: "1"}},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "myproject_data", ConfigHash: "oldhash"},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), alwaysYesPrompt)
	assert.NilError(t, err)

	expected := "[] -> #1 service:db:1, StopContainer, volume data config changed\n" +
		"[1] -> #2 service:db:1, RemoveContainer, volume data config changed\n" +
		"[2] -> #3 volume:data, RemoveVolume, config hash diverged\n" +
		"[3] -> #4 volume:data, CreateVolume, recreate after config change\n"
	assert.Equal(t, plan.String(), expected)
}

func TestReconcileVolumes_DivergedDeclined(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data", Driver: "local"}},
		Services: types.Services{
			"db": {
				Name: "db",
				Volumes: []types.ServiceVolumeConfig{
					{Source: "data", Type: "volume"},
				},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db": {{
				ID: "c1", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c1", Labels: map[string]string{api.ServiceLabel: "db", api.ContainerNumberLabel: "1"}},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "myproject_data", ConfigHash: "oldhash"},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), alwaysNoPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

// --- Helpers ---

func containsLine(s, substr string) bool {
	for _, line := range splitLines(s) {
		if containsStr(line, substr) {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
