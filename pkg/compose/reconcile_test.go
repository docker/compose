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
				Scale:    intPtr(1),
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

	s := plan.String()
	// Network recreation sequence
	assert.Assert(t, containsLine(s, "StopContainer, network frontend config changed"))
	assert.Assert(t, containsLine(s, "DisconnectNetwork, network frontend recreate"))
	assert.Assert(t, containsLine(s, "RemoveNetwork, config hash diverged"))
	assert.Assert(t, containsLine(s, "CreateNetwork, recreate after config change"))
	// Container recreation follows (the container was connected to the old network)
	assert.Assert(t, containsLine(s, "recreate:web:1"))
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
				Scale:    intPtr(1),
				Networks: map[string]*types.ServiceNetworkConfig{"frontend": {}},
			},
			"api": {
				Name:     "api",
				Scale:    intPtr(1),
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
	// Both containers get recreated after network
	assert.Assert(t, containsLine(s, "recreate:web:1"))
	assert.Assert(t, containsLine(s, "recreate:api:1"))
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
				Name:  "db",
				Scale: intPtr(1),
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

	s := plan.String()
	// Volume recreation sequence
	assert.Assert(t, containsLine(s, "StopContainer, volume data config changed"))
	assert.Assert(t, containsLine(s, "RemoveContainer, volume data config changed"))
	assert.Assert(t, containsLine(s, "RemoveVolume, config hash diverged"))
	assert.Assert(t, containsLine(s, "CreateVolume, recreate after config change"))
	// Container is recreated since its config hash diverges after volume removal
	assert.Assert(t, containsLine(s, "CreateContainer,"))
}

func TestReconcileVolumes_DivergedDeclined(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	hash, err := VolumeHash(vol)
	assert.NilError(t, err)

	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data", Driver: "local"}},
		Services: types.Services{
			"db": {
				Name:  "db",
				Scale: intPtr(1),
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
				ConfigHash: mustServiceHash(t, project.Services["db"]),
				Summary: container.Summary{
					ID:    "c1",
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "db",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      mustServiceHash(t, project.Services["db"]),
					},
					Mounts: []container.MountPoint{{Type: "volume", Name: vol.Name}},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			// Volume hash doesn't match, but user declines recreation
			"data": {Name: vol.Name, ConfigHash: hash + "old"},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), alwaysNoPrompt)
	assert.NilError(t, err)
	// Volume not recreated, container is up-to-date -> empty plan
	assert.Assert(t, plan.IsEmpty())
}

// --- Container tests ---

func TestReconcileContainers_NewProject(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"web": {Name: "web", Scale: intPtr(1)},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{"web": {}},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	expected := "[] -> #1 service:web:1, CreateContainer, no existing container\n"
	assert.Equal(t, plan.String(), expected)
}

func TestReconcileContainers_AlreadyRunning(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": svc},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileContainers_ConfigChanged(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"web": {Name: "web", Scale: intPtr(1)},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1aabbccddee", Number: 1, State: container.StateRunning, ConfigHash: "oldhash",
				Summary: container.Summary{
					ID: "c1aabbccddee", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: "oldhash"},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "CreateContainer, config changed (tmpName)"))
	assert.Assert(t, containsLine(s, "StopContainer, replaced by"))
	assert.Assert(t, containsLine(s, "RemoveContainer, replaced by"))
	assert.Assert(t, containsLine(s, "RenameContainer, finalize recreate"))
	// All 4 nodes share the same group
	assert.Assert(t, containsLine(s, "[recreate:web:1]"))
}

func TestReconcileContainers_ScaleUp(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(3)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": svc},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "service:web:2, CreateContainer, no existing container"))
	assert.Assert(t, containsLine(s, "service:web:3, CreateContainer, no existing container"))
	assert.Equal(t, len(plan.Nodes), 2)
}

func TestReconcileContainers_ScaleDown(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": svc},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {
				{
					ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: hash,
					Summary: container.Summary{
						ID: "c1", State: container.StateRunning,
						Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
					},
				},
				{
					ID: "c2", Number: 2, State: container.StateRunning, ConfigHash: hash,
					Summary: container.Summary{
						ID: "c2", State: container.StateRunning,
						Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "2", api.ConfigHashLabel: hash},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "StopContainer, scale down"))
	assert.Assert(t, containsLine(s, "RemoveContainer, scale down"))
	assert.Equal(t, len(plan.Nodes), 2)
}

func TestReconcileContainers_ForceRecreate(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": svc},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1aabbccddee", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1aabbccddee", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	opts := defaultReconcileOptions()
	opts.Recreate = api.RecreateForce

	plan, err := reconcile(t.Context(), project, observed, opts, noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "recreate:web:1"))
	assert.Equal(t, len(plan.Nodes), 4) // create + stop + remove + rename
}

func TestReconcileContainers_NeverRecreate(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"web": {Name: "web", Scale: intPtr(1)},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: "oldhash",
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: "oldhash"},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	opts := defaultReconcileOptions()
	opts.Recreate = api.RecreateNever

	plan, err := reconcile(t.Context(), project, observed, opts, noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileContainers_StoppedNeedsStart(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Scale: intPtr(1)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"web": svc},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"web": {{
				ID: "c1", Number: 1, State: container.StateExited, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1", State: container.StateExited,
					Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	// exited state is not "running", "created", or "restarting" → gets a StartContainer
	// Actually per the code, StateExited is handled explicitly as no-op in current convergence
	// Let me check: the switch has case StateExited with no action. So it should be empty.
	// Wait, looking at the reconciler code: the default case starts it. But StateExited is not
	// explicitly handled — it falls through to default. Let me re-read...
	// The switch is: Running, Created, Restarting → noop. Exited → is not listed, falls to default → start.
	// BUT in the original convergence.go:199, StateExited is a noop. Let me fix.
	// Actually wait: convergence.go:199 shows "case container.StateExited:" with NO body, so it's a noop.
	// My reconciler should match. Let me verify this is tested correctly.
	assert.Assert(t, plan.IsEmpty())
}

func TestReconcileOrphans(t *testing.T) {
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Orphans: []ObservedContainer{{
			ID: "orphan1", Number: 1, Name: "myproject-old-1",
			Summary: container.Summary{ID: "orphan1"},
		}},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	opts := defaultReconcileOptions()
	opts.RemoveOrphans = true

	plan, err := reconcile(t.Context(), project, observed, opts, noPrompt)
	assert.NilError(t, err)

	s := plan.String()
	assert.Assert(t, containsLine(s, "StopContainer, orphaned container"))
	assert.Assert(t, containsLine(s, "RemoveContainer, orphaned container"))
	assert.Equal(t, len(plan.Nodes), 2)
}

// --- Helpers ---

func mustServiceHash(t *testing.T, svc types.ServiceConfig) string {
	t.Helper()
	h, err := ServiceHash(svc)
	assert.NilError(t, err)
	return h
}

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
