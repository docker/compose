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
	"fmt"
	"strconv"
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

// yesPrompt confirms every prompt (equivalent to `--yes`).
func yesPrompt(_ string, _ bool) (bool, error) {
	return true, nil
}

// declinePrompt rejects every prompt (the default answer for a non-interactive
// session with no input).
func declinePrompt(_ string, _ bool) (bool, error) {
	return false, nil
}

// recordingPrompt confirms every prompt and captures the messages shown.
type recordingPrompt struct {
	messages []string
}

func (p *recordingPrompt) confirm(msg string, _ bool) (bool, error) {
	p.messages = append(p.messages, msg)
	return true, nil
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

// divergedVolumeProject builds a project with `count` services (db0, db1, ...),
// each scaled to `scale` and mounting the shared "data" volume, plus a matching
// observed state whose volume config-hash is stale ("oldhash"). Service and
// container config-hashes match, so the only divergence is the volume.
func divergedVolumeProject(t *testing.T, count, scale int) (*types.Project, *ObservedState) {
	t.Helper()
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data": vol},
		Services: types.Services{},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{"data": {Name: vol.Name, ConfigHash: "oldhash"}},
	}
	for s := 0; s < count; s++ {
		name := fmt.Sprintf("db%d", s)
		svc := types.ServiceConfig{
			Name:    name,
			Scale:   intPtr(scale),
			Volumes: []types.ServiceVolumeConfig{{Source: "data", Type: "volume"}},
		}
		project.Services[name] = svc
		hash := mustServiceHash(t, svc)
		for n := 1; n <= scale; n++ {
			id := fmt.Sprintf("%s-%d", name, n)
			observed.Containers[name] = append(observed.Containers[name], ObservedContainer{
				ID: id, Number: n, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: id, State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: name, api.ContainerNumberLabel: strconv.Itoa(n), api.ConfigHashLabel: hash},
					Mounts: []container.MountPoint{{Type: "volume", Name: vol.Name}},
				},
			})
		}
	}
	return project, observed
}

// TestReconcileVolumes_DivergedConfirmed asserts the full recreation sequence for
// a diverged volume mounted by a single service: the container is stopped and
// removed, the volume is removed then recreated, and finally a fresh container is
// scheduled that depends on the new volume.
func TestReconcileVolumes_DivergedConfirmed(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 1)

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db0:1, StopContainer, mounted volume config changed
[1] -> #2 service:db0:1, RemoveContainer, mounted volume config changed
[2] -> #3 volume:data, RemoveVolume, config hash diverged
[3] -> #4 volume:data, CreateVolume, recreate after config change
[4] -> #5 service:db0:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedDeclined verifies that declining the prompt leaves
// the volume (and the service that mounts it) untouched.
func TestReconcileVolumes_DivergedDeclined(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 1)

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), declinePrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "unexpected plan:\n%s", plan.String())
}

// TestReconcileVolumes_DivergedNoRecordedHash verifies that a volume with no
// persisted config-hash (e.g. created by an older Compose) is left untouched and
// never prompts — matching the previous ensureVolume behavior.
func TestReconcileVolumes_DivergedNoRecordedHash(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 1)
	obs := observed.Volumes["data"]
	obs.ConfigHash = ""
	observed.Volumes["data"] = obs

	// noPrompt panics if consulted, proving the empty-hash guard short-circuits.
	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "unexpected plan:\n%s", plan.String())
}

// TestReconcileVolumes_DivergedPromptMessage asserts the confirmation message
// names the volume and warns about data loss.
func TestReconcileVolumes_DivergedPromptMessage(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 1)

	rec := &recordingPrompt{}
	_, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), rec.confirm)
	assert.NilError(t, err)
	assert.Equal(t, len(rec.messages), 1)
	assert.Equal(t, rec.messages[0], `Volume "myproject_data" exists but doesn't match configuration in compose file. Recreate (data will be lost)?`)
}

// TestReconcileVolumes_DivergedPromptError propagates a prompt failure.
func TestReconcileVolumes_DivergedPromptError(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 1)

	boom := func(_ string, _ bool) (bool, error) { return false, fmt.Errorf("boom") }
	_, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), boom)
	assert.ErrorContains(t, err, "boom")
}

// TestReconcileVolumes_DivergedConfirmedScaleN verifies every replica of a
// service mounting the diverged volume is removed, and the same number of fresh
// replicas is recreated after the volume.
func TestReconcileVolumes_DivergedConfirmedScaleN(t *testing.T) {
	project, observed := divergedVolumeProject(t, 1, 2)

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db0:1, StopContainer, mounted volume config changed
[1] -> #2 service:db0:1, RemoveContainer, mounted volume config changed
[] -> #3 service:db0:2, StopContainer, mounted volume config changed
[3] -> #4 service:db0:2, RemoveContainer, mounted volume config changed
[2,4] -> #5 volume:data, RemoveVolume, config hash diverged
[5] -> #6 volume:data, CreateVolume, recreate after config change
[6] -> #7 service:db0:1, CreateContainer, no existing container
[6] -> #8 service:db0:2, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedConfirmedMultipleServices verifies that two
// services mounting the same diverged volume are both recreated, the volume is
// removed only after both services' containers are gone, and both fresh
// containers depend on the single CreateVolume node.
func TestReconcileVolumes_DivergedConfirmedMultipleServices(t *testing.T) {
	project, observed := divergedVolumeProject(t, 2, 1)

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db0:1, StopContainer, mounted volume config changed
[1] -> #2 service:db0:1, RemoveContainer, mounted volume config changed
[] -> #3 service:db1:1, StopContainer, mounted volume config changed
[3] -> #4 service:db1:1, RemoveContainer, mounted volume config changed
[2,4] -> #5 volume:data, RemoveVolume, config hash diverged
[5] -> #6 volume:data, CreateVolume, recreate after config change
[6] -> #7 service:db0:1, CreateContainer, no existing container
[6] -> #8 service:db1:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedConfirmedSharedContainer verifies that a service
// mounting two diverged volumes has its container stopped/removed only once, both
// volumes are recreated, and the fresh container depends on both CreateVolume
// nodes.
func TestReconcileVolumes_DivergedConfirmedSharedContainer(t *testing.T) {
	vol1 := types.VolumeConfig{Name: "myproject_data1", Driver: "local"}
	vol2 := types.VolumeConfig{Name: "myproject_data2", Driver: "local"}
	svc := types.ServiceConfig{
		Name:  "db",
		Scale: intPtr(1),
		Volumes: []types.ServiceVolumeConfig{
			{Source: "data1", Type: "volume"},
			{Source: "data2", Type: "volume"},
		},
	}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data1": vol1, "data2": vol2},
		Services: types.Services{"db": svc},
	}
	hash := mustServiceHash(t, svc)
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: hash,
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "db", api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
					Mounts: []container.MountPoint{{Type: "volume", Name: vol1.Name}, {Type: "volume", Name: vol2.Name}},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data1": {Name: vol1.Name, ConfigHash: "oldhash"},
			"data2": {Name: vol2.Name, ConfigHash: "oldhash"},
		},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db:1, StopContainer, mounted volume config changed
[1] -> #2 service:db:1, RemoveContainer, mounted volume config changed
[2] -> #3 volume:data1, RemoveVolume, config hash diverged
[3] -> #4 volume:data1, CreateVolume, recreate after config change
[2] -> #5 volume:data2, RemoveVolume, config hash diverged
[5] -> #6 volume:data2, CreateVolume, recreate after config change
[4,6] -> #7 service:db:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedPartialConfirm verifies that when several volumes
// diverge but the user confirms only one, only the confirmed volume (and the
// services mounting it) is recreated.
func TestReconcileVolumes_DivergedPartialConfirm(t *testing.T) {
	vol1 := types.VolumeConfig{Name: "myproject_data1", Driver: "local"}
	vol2 := types.VolumeConfig{Name: "myproject_data2", Driver: "local"}
	svc1 := types.ServiceConfig{Name: "db1", Scale: intPtr(1), Volumes: []types.ServiceVolumeConfig{{Source: "data1", Type: "volume"}}}
	svc2 := types.ServiceConfig{Name: "db2", Scale: intPtr(1), Volumes: []types.ServiceVolumeConfig{{Source: "data2", Type: "volume"}}}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data1": vol1, "data2": vol2},
		Services: types.Services{"db1": svc1, "db2": svc2},
	}
	h1, h2 := mustServiceHash(t, svc1), mustServiceHash(t, svc2)
	mountedContainer := func(id, service, hash, volName string) ObservedContainer {
		return ObservedContainer{
			ID: id, Number: 1, State: container.StateRunning, ConfigHash: hash,
			Summary: container.Summary{
				ID: id, State: container.StateRunning,
				Labels: map[string]string{api.ServiceLabel: service, api.ContainerNumberLabel: "1", api.ConfigHashLabel: hash},
				Mounts: []container.MountPoint{{Type: "volume", Name: volName}},
			},
		}
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db1": {mountedContainer("c1", "db1", h1, vol1.Name)},
			"db2": {mountedContainer("c2", "db2", h2, vol2.Name)},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data1": {Name: vol1.Name, ConfigHash: "oldhash"},
			"data2": {Name: vol2.Name, ConfigHash: "oldhash"},
		},
	}

	// Confirm data1 only (sorted order: data1 prompted first).
	first := true
	prompt := func(_ string, _ bool) (bool, error) {
		if first {
			first = false
			return true, nil
		}
		return false, nil
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), prompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:db1:1, StopContainer, mounted volume config changed
[1] -> #2 service:db1:1, RemoveContainer, mounted volume config changed
[2] -> #3 volume:data1, RemoveVolume, config hash diverged
[3] -> #4 volume:data1, CreateVolume, recreate after config change
[4] -> #5 service:db1:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedCascadesToDependent verifies that recreating a
// volume cascades to a dependent that shares the mounting service's mounts via
// volumes_from: the dependent keeps a "container:<id>" reference at runtime, so
// it must be recreated even though its own config is unchanged.
func TestReconcileVolumes_DivergedCascadesToDependent(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	owner := types.ServiceConfig{
		Name:    "owner",
		Image:   "alpine",
		Scale:   intPtr(1),
		Volumes: []types.ServiceVolumeConfig{{Source: "data", Type: "volume"}},
	}
	dependent := types.ServiceConfig{
		Name:        "dependent",
		Image:       "alpine",
		Scale:       intPtr(1),
		VolumesFrom: []string{"owner"},
		DependsOn:   types.DependsOnConfig{"owner": {Condition: types.ServiceConditionStarted, Restart: true, Required: true}},
	}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data": vol},
		Services: types.Services{"owner": owner, "dependent": dependent},
	}

	ownerHash := mustServiceHash(t, owner)
	ownerSummary := container.Summary{
		ID: "owner-1", State: container.StateRunning,
		Labels: map[string]string{api.ServiceLabel: "owner", api.ContainerNumberLabel: "1", api.ConfigHashLabel: ownerHash},
		Mounts: []container.MountPoint{{Type: "volume", Name: vol.Name}},
	}
	dependentHash := mustResolvedServiceHash(t, dependent, map[string]Containers{"owner": {ownerSummary}})
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"owner": {{ID: "owner-1", Number: 1, State: container.StateRunning, ConfigHash: ownerHash, Summary: ownerSummary}},
			"dependent": {{
				ID: "dependent-1", Number: 1, State: container.StateRunning, ConfigHash: dependentHash,
				Summary: container.Summary{
					ID: "dependent-1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "dependent", api.ContainerNumberLabel: "1", api.ConfigHashLabel: dependentHash},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{"data": {Name: vol.Name, ConfigHash: "oldhash"}},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	// The dependent inherits the volume mount via volumes_from, so its container
	// must be stopped and removed before RemoveVolume (#5 depends on both #2 and
	// #4) — otherwise the removal would fail with "volume in use". Both fresh
	// containers are then gated on the new volume: owner (#7) depends on
	// CreateVolume (#6), and the dependent (#8) depends on owner (#7).
	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 service:dependent:1, StopContainer, mounted volume config changed
[1] -> #2 service:dependent:1, RemoveContainer, mounted volume config changed
[] -> #3 service:owner:1, StopContainer, mounted volume config changed
[3] -> #4 service:owner:1, RemoveContainer, mounted volume config changed
[2,4] -> #5 volume:data, RemoveVolume, config hash diverged
[5] -> #6 volume:data, CreateVolume, recreate after config change
[6] -> #7 service:owner:1, CreateContainer, no existing container
[7] -> #8 service:dependent:1, CreateContainer, no existing container
`)+"\n")
}

// TestReconcileVolumes_DivergedVolumesFromRemovedBeforeVolume specifically guards
// against a "volume in use" failure: a service reaching the diverged volume only
// through volumes_from (never mounting it directly) must still have its container
// removed before the volume is removed.
func TestReconcileVolumes_DivergedVolumesFromRemovedBeforeVolume(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	owner := types.ServiceConfig{
		Name:    "owner",
		Image:   "alpine",
		Scale:   intPtr(1),
		Volumes: []types.ServiceVolumeConfig{{Source: "data", Type: "volume"}},
	}
	// consumer inherits owner's mounts (including data) but never declares the
	// volume itself.
	consumer := types.ServiceConfig{
		Name:        "consumer",
		Image:       "alpine",
		Scale:       intPtr(1),
		VolumesFrom: []string{"owner"},
		DependsOn:   types.DependsOnConfig{"owner": {Condition: types.ServiceConditionStarted, Required: true}},
	}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data": vol},
		Services: types.Services{"owner": owner, "consumer": consumer},
	}
	ownerHash := mustServiceHash(t, owner)
	ownerSummary := container.Summary{
		ID: "owner-1", State: container.StateRunning,
		Labels: map[string]string{api.ServiceLabel: "owner", api.ContainerNumberLabel: "1", api.ConfigHashLabel: ownerHash},
		Mounts: []container.MountPoint{{Type: "volume", Name: vol.Name}},
	}
	consumerHash := mustResolvedServiceHash(t, consumer, map[string]Containers{"owner": {ownerSummary}})
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"owner": {{ID: "owner-1", Number: 1, State: container.StateRunning, ConfigHash: ownerHash, Summary: ownerSummary}},
			"consumer": {{
				ID: "consumer-1", Number: 1, State: container.StateRunning, ConfigHash: consumerHash,
				Summary: container.Summary{
					ID: "consumer-1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "consumer", api.ContainerNumberLabel: "1", api.ConfigHashLabel: consumerHash},
					// Docker materializes the inherited mount on the consumer.
					Mounts: []container.MountPoint{{Type: "volume", Name: vol.Name}},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{"data": {Name: vol.Name, ConfigHash: "oldhash"}},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	planStr := plan.String()
	// The volumes_from consumer is stopped and removed as part of the volume
	// recreation batch, even though it never declares the volume.
	assert.Assert(t, strings.Contains(planStr, "service:consumer:1, RemoveContainer, mounted volume config changed"),
		"volumes_from consumer must be removed before RemoveVolume:\n%s", planStr)

	// RemoveVolume must depend on the consumer's RemoveContainer node.
	var removeConsumer, removeVolume *PlanNode
	for _, n := range plan.Nodes {
		if n.Operation.Type == OpRemoveContainer && n.Operation.ResourceID == "service:consumer:1" {
			removeConsumer = n
		}
		if n.Operation.Type == OpRemoveVolume {
			removeVolume = n
		}
	}
	assert.Assert(t, removeConsumer != nil, "no RemoveContainer for consumer:\n%s", planStr)
	assert.Assert(t, removeVolume != nil, "no RemoveVolume:\n%s", planStr)
	found := false
	for _, dep := range removeVolume.DependsOn {
		if dep == removeConsumer {
			found = true
			break
		}
	}
	assert.Assert(t, found, "RemoveVolume must depend on the consumer's RemoveContainer:\n%s", planStr)
}

// TestReconcileVolumes_UnmanagedMatchReused verifies that a volume discovered by
// name but not owned by the project (empty ConfigHash — see collectObservedState)
// is reused untouched: no create, no recreation, and no prompt.
func TestReconcileVolumes_UnmanagedMatchReused(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data", Driver: "local"}},
		Services: types.Services{
			"db": {Name: "db", Scale: intPtr(1), Volumes: []types.ServiceVolumeConfig{{Source: "data", Type: "volume"}}},
		},
	}
	dbHash := mustServiceHash(t, project.Services["db"])
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers: map[string][]ObservedContainer{
			"db": {{
				ID: "c1", Number: 1, State: container.StateRunning, ConfigHash: dbHash,
				Summary: container.Summary{
					ID: "c1", State: container.StateRunning,
					Labels: map[string]string{api.ServiceLabel: "db", api.ContainerNumberLabel: "1", api.ConfigHashLabel: dbHash},
					Mounts: []container.MountPoint{{Type: "volume", Name: "myproject_data"}},
				},
			}},
		},
		Networks: map[string]ObservedNetwork{},
		// Unmanaged match: name resolved, but no config hash recorded.
		Volumes: map[string]ObservedVolume{"data": {Name: "myproject_data", ConfigHash: ""}},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "unmanaged volume must be reused untouched:\n%s", plan.String())
}

// TestReconcileVolumes_RenamedIsAdditive verifies that renaming a volume (the
// label-matched live volume carries a different name) creates the new volume and
// leaves the old one — and its data — untouched, without prompting.
func TestReconcileVolumes_RenamedIsAdditive(t *testing.T) {
	project := &types.Project{
		Name:    "myproject",
		Volumes: types.Volumes{"data": {Name: "myproject_data_v2", Driver: "local"}},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		// Same compose key "data", but the live volume still has the old name.
		Volumes: map[string]ObservedVolume{
			"data": {Name: "myproject_data", ConfigHash: mustVolumeHash(t, types.VolumeConfig{Name: "myproject_data", Driver: "local"})},
		},
	}

	// noPrompt: a rename must not prompt for destructive recreation.
	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), noPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 volume:data, CreateVolume, renamed
`)+"\n")
}

// TestReconcileVolumes_DivergedUnmountedVolume verifies that a diverged volume
// declared by the project but mounted by no running container is still recreated
// (no container operations, just remove + create).
func TestReconcileVolumes_DivergedUnmountedVolume(t *testing.T) {
	vol := types.VolumeConfig{Name: "myproject_data", Driver: "local"}
	project := &types.Project{
		Name:     "myproject",
		Volumes:  types.Volumes{"data": vol},
		Services: types.Services{},
	}
	observed := &ObservedState{
		ProjectName: "myproject",
		Containers:  map[string][]ObservedContainer{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{"data": {Name: vol.Name, ConfigHash: "oldhash"}},
	}

	plan, err := reconcile(t.Context(), project, observed, defaultReconcileOptions(), yesPrompt)
	assert.NilError(t, err)

	assert.Equal(t, plan.String(), strings.TrimSpace(`
[] -> #1 volume:data, RemoveVolume, config hash diverged
[1] -> #2 volume:data, CreateVolume, recreate after config change
`)+"\n")
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

func mustVolumeHash(t *testing.T, vol types.VolumeConfig) string {
	t.Helper()
	h, err := VolumeHash(vol)
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
