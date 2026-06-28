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
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// noPrompt is a Prompt that should never be called in tests that don't expect it.
func noPrompt(msg string, _ bool) (bool, error) {
	panic("unexpected prompt call: " + msg)
}

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

	// Sorted alphabetically: backend before frontend
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 network:backend, CreateNetwork, not found
[] -> #2 network:frontend, CreateNetwork, not found
`)+"\n")
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
			"web": {{
				ID: "c1aabbccddee", Number: 1, State: container.StateRunning,
				Summary: container.Summary{
					ID: "c1aabbccddee",
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
					},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"},
		},
		Volumes: map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	// The recreate phase reuses the Stop from the network-recreate phase
	// instead of emitting a second one against an already-stopped container.
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, StopContainer, network frontend config changed
[1] -> #2 service:web:1, DisconnectNetwork, network frontend recreate
[2] -> #3 network:frontend, RemoveNetwork, config hash diverged
[3] -> #4 network:frontend, CreateNetwork, recreate after config change
[4] -> #5 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[1,5] -> #6 service:web:1, RemoveContainer, replaced by #5 [recreate:web:1]
[6] -> #7 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")
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
				ID: "c1aabbccddee", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c1aabbccddee", Labels: map[string]string{api.ServiceLabel: "web", api.ContainerNumberLabel: "1"}},
			}},
			"api": {{
				ID: "c2aabbccddee", Number: 1, State: container.StateRunning,
				Summary: container.Summary{ID: "c2aabbccddee", Labels: map[string]string{api.ServiceLabel: "api", api.ContainerNumberLabel: "1"}},
			}},
		},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net1", Name: "myproject_frontend", ConfigHash: "oldhash"},
		},
		Volumes: map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	// Services sorted alphabetically: api before web. Each service's recreate
	// reuses the Stop from the network-recreate phase (no second Stop).
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:api:1, StopContainer, network frontend config changed
[] -> #2 service:web:1, StopContainer, network frontend config changed
[1] -> #3 service:api:1, DisconnectNetwork, network frontend recreate
[2] -> #4 service:web:1, DisconnectNetwork, network frontend recreate
[3,4] -> #5 network:frontend, RemoveNetwork, config hash diverged
[5] -> #6 network:frontend, CreateNetwork, recreate after config change
[6] -> #7 service:api:1, CreateContainer, config changed (tmpName) [recreate:api:1]
[1,7] -> #8 service:api:1, RemoveContainer, replaced by #7 [recreate:api:1]
[8] -> #9 service:api:1, RenameContainer, finalize recreate [recreate:api:1]
[6] -> #10 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[2,10] -> #11 service:web:1, RemoveContainer, replaced by #10 [recreate:web:1]
[11] -> #12 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 volume:data, CreateVolume, not found
`)+"\n")
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

// TestReconcileVolumes_DivergedIsIgnored verifies that a diverged volume
// produces no plan operations: recreation of diverged volumes is owned by
// ensureProjectVolumes (which prompts the user) and runs before reconcile,
// so the reconciler must not duplicate that decision.
func TestReconcileVolumes_DivergedIsIgnored(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}

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
			"data": {Name: vol.Name, ConfigHash: "oldhash"},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, CreateContainer, no existing container
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[1] -> #2 service:web:1, StopContainer, replaced by #1 [recreate:web:1]
[2] -> #3 service:web:1, RemoveContainer, replaced by #1 [recreate:web:1]
[3] -> #4 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:2, CreateContainer, no existing container
[] -> #2 service:web:3, CreateContainer, no existing container
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:2, StopContainer, scale down
[1] -> #2 service:web:2, RemoveContainer, scale down
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:web:1, CreateContainer, config changed (tmpName) [recreate:web:1]
[1] -> #2 service:web:1, StopContainer, replaced by #1 [recreate:web:1]
[2] -> #3 service:web:1, RemoveContainer, replaced by #1 [recreate:web:1]
[3] -> #4 service:web:1, RenameContainer, finalize recreate [recreate:web:1]
`)+"\n")
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

func TestReconcileContainers_ExitedIsNoop(t *testing.T) {
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
	// Exited containers are left as-is, matching convergence.go:199 behavior
	assert.Assert(t, plan.IsEmpty())
}

// TestReconcileContainers_DependsOnChain verifies that a dependent service's
// container creation depends on the last plan node of the service it depends
// on (via reconciler.serviceNodes). Without this, services declared in
// depends_on could start before their dependencies' operations complete.
func TestReconcileContainers_DependsOnChain(t *testing.T) {
	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1)},
			"web": {
				Name:  "web",
				Scale: intPtr(1),
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionStarted},
				},
			},
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

	// db has no dependencies, so its CreateContainer has no deps.
	// web depends on db, so its CreateContainer must depend on db's node #1.
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, CreateContainer, no existing container
[1] -> #2 service:web:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileContainers_DependsOnScaleDown verifies that scale-down of a
// service still propagates through serviceNodes, so a dependent service waits
// for the scale-down's RemoveContainer to finish before starting its own ops.
func TestReconcileContainers_DependsOnScaleDown(t *testing.T) {
	svc := types.ServiceConfig{Name: "db", Scale: intPtr(0)}
	hash := mustServiceHash(t, svc)

	project := &types.Project{
		Name: "myproject",
		Services: types.Services{
			"db": svc,
			"web": {
				Name:  "web",
				Scale: intPtr(1),
				DependsOn: types.DependsOnConfig{
					"db": {Condition: types.ServiceConditionStarted},
				},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "db", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	// db scales 1→0: Stop+Remove. web's CreateContainer must depend on the
	// Remove (#2), proving serviceNodes is updated even when only scale-down
	// happens for the dependency.
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, StopContainer, scale down
[1] -> #2 service:db:1, RemoveContainer, scale down
[2] -> #3 service:web:1, CreateContainer, no existing container
`)+"\n")
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

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 orphan:myproject-old-1, StopContainer, orphaned container
[1] -> #2 orphan:myproject-old-1, RemoveContainer, orphaned container
`)+"\n")
}

// --- Helpers ---

func mustServiceHash(t *testing.T, svc types.ServiceConfig) string {
	t.Helper()
	h, err := ServiceHash(svc)
	assert.NilError(t, err)
	return h
}
