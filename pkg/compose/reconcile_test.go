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

// --- Service-reference resolution tests (issue #13878) ---
//
// When a service uses network_mode/ipc/pid: service:X or volumes_from: serviceName,
// the executor mutates the resolved field to "container:<id>" (or bare ID for
// volumes_from) before computing and persisting the config-hash. The reconciler
// must hash the same resolved form so that an unchanged config does not appear
// as a divergence on the next `up`.

// parentDependentObserved builds an ObservedState containing a "parent" container
// (running, with its own raw hash) and a "dependent" container whose hash was
// computed on the resolved form of its service.
func parentDependentObserved(t *testing.T, parent, dependent types.ServiceConfig) *ObservedState {
	t.Helper()
	const parentID = "parent_container_abc123"
	parentSummary := container.Summary{
		ID: parentID, State: container.StateRunning,
		Labels: map[string]string{
			api.ServiceLabel:         "parent",
			api.ContainerNumberLabel: "1",
			api.ConfigHashLabel:      mustServiceHash(t, parent),
		},
	}
	parentObserved := []ObservedContainer{{
		ID: parentID, Number: 1, State: container.StateRunning,
		ConfigHash: mustServiceHash(t, parent),
		Summary:    parentSummary,
	}}
	containersByService := map[string]Containers{"parent": {parentSummary}}
	dependentHash := mustResolvedServiceHash(t, dependent, containersByService)
	return &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"parent": parentObserved,
			"dependent": {{
				ID: "dependent_container_xyz", Number: 1, State: container.StateRunning,
				ConfigHash: dependentHash,
				Summary: container.Summary{
					ID: "dependent_container_xyz", State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "dependent",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      dependentHash,
					},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
	}
}

// TestReconcileContainers_ServiceReference_NoRecreate covers every reference
// form mutated by resolveServiceReferences. The volumes_from "container:X"
// form is included because resolveVolumeFrom strips the prefix — the persisted
// hash differs from the raw user form even though no service reference is
// involved.
func TestReconcileContainers_ServiceReference_NoRecreate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*types.ServiceConfig)
	}{
		{"network_mode_service", func(s *types.ServiceConfig) { s.NetworkMode = "service:parent" }},
		{"ipc_service", func(s *types.ServiceConfig) { s.Ipc = "service:parent" }},
		{"pid_service", func(s *types.ServiceConfig) { s.Pid = "service:parent" }},
		{"volumes_from_service", func(s *types.ServiceConfig) { s.VolumesFrom = []string{"parent"} }},
		{"volumes_from_container", func(s *types.ServiceConfig) { s.VolumesFrom = []string{"container:some_external"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := types.ServiceConfig{Name: "parent", Image: "alpine", Scale: intPtr(1)}
			dependent := types.ServiceConfig{Name: "dependent", Image: "alpine", Scale: intPtr(1)}
			tc.mutate(&dependent)
			project := &types.Project{
				Name:     "myproject",
				Services: types.Services{"parent": parent, "dependent": dependent},
			}
			plan, err := reconcile(t.Context(), project, parentDependentObserved(t, parent, dependent), defaultReconcileOptions(), noPrompt)
			assert.NilError(t, err)
			assert.Assert(t, plan.IsEmpty(), "unexpected plan:\n%s", plan.String())
		})
	}
}

// TestReconcileContainers_NamespaceParentRecreated_CascadesToDependent verifies
// that when a parent service is scheduled for recreation, dependents sharing
// its namespace (network_mode: service:X) are also recreated. Otherwise the
// dependent would keep a stale "container:<old_id>" reference at runtime.
// The implicit depends_on {restart: true} matches what compose-go's normalizer
// produces for namespace-sharing services, so planStopDependents fires too —
// the test also asserts the resulting Stop is not duplicated.
func TestReconcileContainers_NamespaceParentRecreated_CascadesToDependent(t *testing.T) {
	parent := types.ServiceConfig{Name: "parent", Image: "alpine", Scale: intPtr(1)}
	dependent := types.ServiceConfig{
		Name: "dependent", Image: "alpine", Scale: intPtr(1), NetworkMode: "service:parent",
		DependsOn: types.DependsOnConfig{"parent": {Condition: types.ServiceConditionStarted, Restart: true, Required: true}},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"parent": parent, "dependent": dependent},
	}
	observed := parentDependentObserved(t, parent, dependent)
	observed.Containers["parent"][0].ConfigHash = "stale_parent_hash"
	observed.Containers["parent"][0].Summary.Labels[api.ConfigHashLabel] = "stale_parent_hash"

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	planStr := plan.String()
	assert.Assert(t, strings.Contains(planStr, "service:parent:1, CreateContainer"), "parent must be recreated:\n%s", planStr)
	assert.Assert(t, strings.Contains(planStr, "service:dependent:1, CreateContainer"), "dependent must cascade-recreate:\n%s", planStr)
	// One Stop per container — planStopDependents must not duplicate the Stop
	// emitted by planRecreateContainer for the dependent.
	assert.Equal(t, strings.Count(planStr, "service:dependent:1, StopContainer"), 1, "duplicate Stop for dependent:\n%s", planStr)
}

// TestReconcileContainers_MultipleParents_EitherTriggersCascade guards
// parentNamespaceRecreated against a future refactor that early-returns on a
// non-matching parent: a dependent sharing namespace with two parents must
// cascade-recreate when either parent is scheduled for recreation.
func TestReconcileContainers_MultipleParents_EitherTriggersCascade(t *testing.T) {
	netParent := types.ServiceConfig{Name: "netparent", Image: "alpine", Scale: intPtr(1)}
	volParent := types.ServiceConfig{Name: "volparent", Image: "alpine", Scale: intPtr(1)}
	dependent := types.ServiceConfig{
		Name: "dependent", Image: "alpine", Scale: intPtr(1),
		NetworkMode: "service:netparent",
		VolumesFrom: []string{"volparent"},
		// Mirrors what compose-go's normalizer injects for namespace-sharing
		// references, so the dependency graph orders parents before dependent.
		DependsOn: types.DependsOnConfig{
			"netparent": {Condition: types.ServiceConditionStarted, Restart: true, Required: true},
			"volparent": {Condition: types.ServiceConditionStarted, Restart: true, Required: true},
		},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"netparent": netParent, "volparent": volParent, "dependent": dependent},
	}

	// Build a containersByService map containing both parents so the
	// dependent's resolved hash can be computed.
	parents := map[string]Containers{
		"netparent": {container.Summary{ID: "netparent_id", Labels: map[string]string{api.ServiceLabel: "netparent"}}},
		"volparent": {container.Summary{ID: "volparent_id", Labels: map[string]string{api.ServiceLabel: "volparent"}}},
	}
	dependentHash := mustResolvedServiceHash(t, dependent, parents)

	makeObserved := func(staleService string) *ObservedState {
		obs := &ObservedState{
			ProjectName: "myproject",
			Containers:  map[string][]ObservedContainer{},
			Networks:    map[string]ObservedNetwork{},
			Volumes:     map[string]ObservedVolume{},
		}
		for name, svc := range map[string]types.ServiceConfig{"netparent": netParent, "volparent": volParent} {
			hash := mustServiceHash(t, svc)
			if name == staleService {
				hash = "stale"
			}
			obs.Containers[name] = []ObservedContainer{{
				ID: name + "_id", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: name + "_id", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: name, api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				},
			}}
		}
		obs.Containers["dependent"] = []ObservedContainer{{
			ID: "dependent_id", Number: 1, State: container.StateRunning, ConfigHash: dependentHash,
			Summary: container.Summary{
				ID: "dependent_id", State: container.StateRunning,
				Labels: map[string]string{api.ServiceLabel: "dependent", api.ContainerNumberLabel: "1", api.ConfigHashLabel: dependentHash},
			},
		}}
		return obs
	}

	for _, staleParent := range []string{"netparent", "volparent"} {
		t.Run("stale_"+staleParent, func(t *testing.T) {
			plan, err := reconcile(t.Context(), project, makeObserved(staleParent), defaultReconcileOptions(), noPrompt)
			assert.NilError(t, err)
			planStr := plan.String()
			assert.Assert(t, strings.Contains(planStr, "service:dependent:1, CreateContainer"), "dependent must cascade-recreate when %s is recreated:\n%s", staleParent, planStr)
		})
	}
}

// TestReconcileContainers_RegularDependsOn_NoCascade ensures the cascade fires
// only for namespace/volume-sharing dependencies, not for plain depends_on.
func TestReconcileContainers_RegularDependsOn_NoCascade(t *testing.T) {
	parent := types.ServiceConfig{Name: "parent", Image: "alpine", Scale: intPtr(1)}
	dependent := types.ServiceConfig{
		Name: "dependent", Image: "alpine", Scale: intPtr(1),
		DependsOn: types.DependsOnConfig{"parent": {Condition: types.ServiceConditionStarted, Restart: true}},
	}
	project := &types.Project{
		Name:     "myproject",
		Services: types.Services{"parent": parent, "dependent": dependent},
	}
	observed := parentDependentObserved(t, parent, dependent)
	observed.Containers["parent"][0].ConfigHash = "stale_parent_hash"
	observed.Containers["parent"][0].Summary.Labels[api.ConfigHashLabel] = "stale_parent_hash"

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	planStr := plan.String()
	assert.Assert(t, strings.Contains(planStr, "service:parent:1, CreateContainer"), "parent must be recreated:\n%s", planStr)
	assert.Assert(t, !strings.Contains(planStr, "service:dependent:1, CreateContainer"), "dependent must NOT recreate without namespace sharing:\n%s", planStr)
}

// --- Helpers ---

func mustServiceHash(t *testing.T, svc types.ServiceConfig) string {
	t.Helper()
	h, err := ServiceHash(svc)
	assert.NilError(t, err)
	return h
}

// mustResolvedServiceHash mirrors what the executor persists at create time:
// the service references are resolved before hashing. Use it to seed
// ObservedContainer.ConfigHash in tests involving network_mode/ipc/pid:
// service:X or volumes_from: serviceName.
func mustResolvedServiceHash(t *testing.T, svc types.ServiceConfig, containers map[string]Containers) string {
	t.Helper()
	h, err := serviceHashWithResolvedRefs(svc, containers)
	assert.NilError(t, err)
	return h
}
