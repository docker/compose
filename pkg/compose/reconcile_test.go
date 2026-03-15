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
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// ---------------------------------------------------------------------------
// needsRecreate tests
// ---------------------------------------------------------------------------

func TestNeedsRecreateNeverPolicy(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	ctr := container.Summary{
		ID:     "abc123",
		Names:  []string{"/testproject-web-1"},
		Labels: map[string]string{},
		State:  container.StateRunning,
	}

	recreate, reason, err := needsRecreate(service, ctr, nil, nil, api.RecreateNever)
	assert.NilError(t, err)
	assert.Assert(t, !recreate)
	assert.Equal(t, reason, "")
}

func TestNeedsRecreateForcePolicy(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	ctr := container.Summary{
		ID:     "abc123",
		Names:  []string{"/testproject-web-1"},
		Labels: map[string]string{},
		State:  container.StateRunning,
	}

	recreate, reason, err := needsRecreate(service, ctr, nil, nil, api.RecreateForce)
	assert.NilError(t, err)
	assert.Assert(t, recreate)
	assert.Equal(t, reason, "force recreate")
}

func TestNeedsRecreateConfigHashChanged(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	ctr := container.Summary{
		ID:    "abc123",
		Names: []string{"/testproject-web-1"},
		Labels: map[string]string{
			api.ConfigHashLabel: "stale-hash-value",
		},
		State: container.StateRunning,
	}

	recreate, reason, err := needsRecreate(service, ctr, nil, nil, api.RecreateDiverged)
	assert.NilError(t, err)
	assert.Assert(t, recreate)
	assert.Equal(t, reason, "config hash changed")
}

func TestNeedsRecreateUpToDate(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(service)
	assert.NilError(t, err)

	ctr := container.Summary{
		ID:    "abc123",
		Names: []string{"/testproject-web-1"},
		Labels: map[string]string{
			api.ConfigHashLabel:  hash,
			api.ImageDigestLabel: "", // matches zero-value in CustomLabels
		},
		State: container.StateRunning,
	}

	recreate, reason, err := needsRecreate(service, ctr, nil, nil, api.RecreateDiverged)
	assert.NilError(t, err)
	assert.Assert(t, !recreate)
	assert.Equal(t, reason, "")
}

// ---------------------------------------------------------------------------
// Reconcile tests
// ---------------------------------------------------------------------------

func TestReconcileCreateMissingNetwork(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "testproject_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Networks:
  create     testproject_default  "network does not exist"
Containers:
  create     testproject-web-1  "scale up"
`)
}

func TestReconcileSkipUpToDateNetwork(t *testing.T) {
	net := types.NetworkConfig{Name: "testproject_default"}
	hash, err := NetworkHash(&net)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Networks: types.Networks{
			"default": net,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks: map[string]ObservedNetwork{
			"default": {
				ID:         "net123",
				Name:       "testproject_default",
				ConfigHash: hash,
			},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpCreateNetwork || op.Type == OpRecreateNetwork {
			t.Fatalf("unexpected network operation: %s", op.ID)
		}
	}
}

func TestReconcileRecreateChangedNetwork(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "testproject_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks: map[string]ObservedNetwork{
			"default": {
				ID:         "net123",
				Name:       "testproject_default",
				ConfigHash: "outdated-hash",
			},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Networks:
  recreate   testproject_default  "config hash changed"
Containers:
  create     testproject-web-1  "scale up"
`)
}

func TestReconcileCreateMissingVolume(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Volumes: types.Volumes{
			"data": types.VolumeConfig{Name: "testproject_data"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Volumes:
  create     testproject_data  "volume does not exist"
Containers:
  create     testproject-web-1  "scale up"
`)
}

func TestReconcileScaleUp(t *testing.T) {
	service := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Scale: intPtr(2),
	}
	// Compute hash before project is built, since ServiceHash strips Scale
	hash, err := ServiceHash(service)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": service,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123",
					Names: []string{"/testproject-web-1"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	createCount := 0
	for _, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ServiceName == "web" {
			createCount++
			assert.Equal(t, op.Reason, "scale up")
		}
	}
	assert.Assert(t, createCount >= 1, "expected at least one OpCreateContainer for scale up, plan:\n%s", plan.String())
}

func TestReconcileScaleDown(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(service)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": service,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123",
					Names: []string{"/testproject-web-1"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
				{
					ID:    "def456",
					Names: []string{"/testproject-web-2"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "2",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	hasStop := false
	hasRemove := false
	for _, op := range plan.Operations {
		if op.Type == OpStopContainer && op.Reason == "scale down" {
			hasStop = true
		}
		if op.Type == OpRemoveContainer && op.Reason == "scale down" {
			hasRemove = true
		}
	}
	assert.Assert(t, hasStop, "expected OpStopContainer for scale down")
	assert.Assert(t, hasRemove, "expected OpRemoveContainer for scale down")
}

func TestReconcileRecreateContainer(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": service,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123",
					Names: []string{"/testproject-web-1"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      "stale-hash",
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Containers:
  recreate   testproject-web-1  "config hash changed"
`)
}

func TestReconcileNoChanges(t *testing.T) {
	service := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(service)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": service,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123",
					Names: []string{"/testproject-web-1"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

func TestReconcileOrphans(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans: Containers{
			{
				ID:    "orphan1",
				Names: []string{"/testproject-old-1"},
				Labels: map[string]string{
					api.ServiceLabel:         "old",
					api.ContainerNumberLabel: "1",
					api.ProjectLabel:         "testproject",
				},
				State: container.StateRunning,
			},
		},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Containers:
  create     testproject-web-1  "scale up"
  remove     testproject-old-1  "orphan container"
    depends on: stop-container:testproject-old-1
  stop       testproject-old-1  "orphan container"
`)
}

func TestReconcilePluginService(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"plugin-svc": types.ServiceConfig{
				Name: "plugin-svc",
				Provider: &types.ServiceProviderConfig{
					Type: "aws",
				},
			},
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Plugins:
  plugin     plugin-svc  "plugin service"
`)
}

func TestReconcileDependencyEdges(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"db": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"db": types.ServiceConfig{
				Name:  "db",
				Image: "postgres",
			},
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// Find the create-container op for "web" and verify it depends on the "db" create op
	var webCreateOp *Operation
	var dbCreateID string
	for _, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ServiceName == "web" {
			webCreateOp = op
		}
		if op.Type == OpCreateContainer && op.ServiceName == "db" {
			dbCreateID = op.ID
		}
	}

	assert.Assert(t, webCreateOp != nil, "expected create-container op for web")
	assert.Assert(t, dbCreateID != "", "expected create-container op for db")

	assert.Assert(t, slices.Contains(webCreateOp.DependsOn, dbCreateID), "web create op should depend on db create op, got deps: %v", webCreateOp.DependsOn)
}

// ---------------------------------------------------------------------------
// Tests mirroring e2e scenarios — pure Reconcile tests that cover the same
// decision logic as the corresponding e2e/integration tests.
// ---------------------------------------------------------------------------

// TestReconcileScaleUpMultipleServices mirrors e2e TestScaleBasicCases:
// scaling up 2 services simultaneously should produce create ops for each.
func TestReconcileScaleUpMultipleServices(t *testing.T) {
	frontSvc := types.ServiceConfig{Name: "front", Image: "nginx", Scale: intPtr(3)}
	backSvc := types.ServiceConfig{Name: "back", Image: "nginx", Scale: intPtr(2)}
	frontHash, err := ServiceHash(frontSvc)
	assert.NilError(t, err)
	backHash, err := ServiceHash(backSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "scale-basic-tests",
		Services: types.Services{
			"front": frontSvc,
			"back":  backSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "scale-basic-tests",
		Containers: map[string]Containers{
			"front": {
				makeContainer("scale-basic-tests", "front", 1, frontHash),
			},
			"back": {
				makeContainer("scale-basic-tests", "back", 1, backHash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	frontCreates := countOps(plan, OpCreateContainer, "front")
	backCreates := countOps(plan, OpCreateContainer, "back")
	assert.Equal(t, frontCreates, 2, "expected 2 create ops for front (scale 1→3)")
	assert.Equal(t, backCreates, 1, "expected 1 create op for back (scale 1→2)")
}

// TestReconcileScaleDownMultipleServices mirrors e2e TestScaleBasicCases:
// scaling down 2 services simultaneously should produce stop+remove ops.
func TestReconcileScaleDownMultipleServices(t *testing.T) {
	frontSvc := types.ServiceConfig{Name: "front", Image: "nginx", Scale: intPtr(2)}
	backSvc := types.ServiceConfig{Name: "back", Image: "nginx", Scale: intPtr(1)}
	frontHash, err := ServiceHash(frontSvc)
	assert.NilError(t, err)
	backHash, err := ServiceHash(backSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "scale-basic-tests",
		Services: types.Services{
			"front": frontSvc,
			"back":  backSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "scale-basic-tests",
		Containers: map[string]Containers{
			"front": {
				makeContainer("scale-basic-tests", "front", 1, frontHash),
				makeContainer("scale-basic-tests", "front", 2, frontHash),
				makeContainer("scale-basic-tests", "front", 3, frontHash),
			},
			"back": {
				makeContainer("scale-basic-tests", "back", 1, backHash),
				makeContainer("scale-basic-tests", "back", 2, backHash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	frontStops := countOps(plan, OpStopContainer, "front")
	frontRemoves := countOps(plan, OpRemoveContainer, "front")
	backStops := countOps(plan, OpStopContainer, "back")
	backRemoves := countOps(plan, OpRemoveContainer, "back")
	assert.Equal(t, frontStops, 1, "expected 1 stop for front (scale 3→2)")
	assert.Equal(t, frontRemoves, 1, "expected 1 remove for front (scale 3→2)")
	assert.Equal(t, backStops, 1, "expected 1 stop for back (scale 2→1)")
	assert.Equal(t, backRemoves, 1, "expected 1 remove for back (scale 2→1)")
}

// TestReconcileScaleToZero mirrors part of e2e TestScaleBasicCases:
// scaling a service to 0 should stop+remove all its containers.
func TestReconcileScaleToZero(t *testing.T) {
	svc := types.ServiceConfig{Name: "dbadmin", Image: "nginx", Scale: intPtr(0)}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"dbadmin": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"dbadmin": {
				makeContainer("testproject", "dbadmin", 1, hash),
				makeContainer("testproject", "dbadmin", 2, hash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	stops := countOps(plan, OpStopContainer, "dbadmin")
	removes := countOps(plan, OpRemoveContainer, "dbadmin")
	assert.Equal(t, stops, 2, "expected 2 stop ops to scale to 0")
	assert.Equal(t, removes, 2, "expected 2 remove ops to scale to 0")
}

// TestReconcileScaleDownRemovesObsoleteFirst mirrors e2e TestScaleDownRemovesObsolete:
// when scaling down and some containers are obsolete (stale hash), the obsolete
// ones should be removed first (via stop+remove), keeping the up-to-date ones.
func TestReconcileScaleDownRemovesObsoleteFirst(t *testing.T) {
	svc := types.ServiceConfig{Name: "db", Image: "postgres"}
	currentHash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"db": svc,
		},
	}

	// Container 1 has a stale hash (obsolete), container 2 is up-to-date.
	// Sorting puts obsolete containers first for removal during scale down.
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"db": {
				makeContainer("testproject", "db", 1, "stale-hash-obsolete"),
				makeContainer("testproject", "db", 2, currentHash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// With scale=1 (default) and 2 containers: the obsolete container is
	// sorted first and gets scale-downed (stop+remove). The remaining
	// up-to-date container stays (no recreate needed).
	stops := countOps(plan, OpStopContainer, "db")
	removes := countOps(plan, OpRemoveContainer, "db")
	assert.Equal(t, stops, 1, "expected 1 stop for scale down of obsolete container")
	assert.Equal(t, removes, 1, "expected 1 remove for scale down of obsolete container")

	// The stop/remove should target the obsolete container (stale hash),
	// not the up-to-date one
	for _, op := range plan.Operations {
		if op.Type == OpStopContainer && op.ServiceName == "db" {
			assert.Assert(t, op.ContainerOp.Existing != nil)
			assert.Equal(t, op.ContainerOp.Existing.Labels[api.ConfigHashLabel], "stale-hash-obsolete",
				"obsolete container should be removed first during scale down")
		}
	}
}

// TestReconcileScaleUpNoRecreate mirrors e2e TestScaleDoesntRecreate and
// TestScaleDownNoRecreate: scaling up with --no-recreate should only create
// new containers, not recreate existing ones even if their image has changed.
func TestReconcileScaleUpNoRecreate(t *testing.T) {
	svc := types.ServiceConfig{Name: "test", Image: "nginx", Scale: intPtr(4)}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"test": svc,
		},
	}

	// 2 existing containers with a stale hash (image was rebuilt)
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"test": {
				makeContainer("testproject", "test", 1, "old-hash-before-rebuild"),
				makeContainer("testproject", "test", 2, "old-hash-before-rebuild"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateNever, // --no-recreate
		RecreateDependencies: api.RecreateNever,
	})
	assert.NilError(t, err)

	creates := countOps(plan, OpCreateContainer, "test")
	recreates := countOps(plan, OpRecreateContainer, "test")
	assert.Equal(t, creates, 2, "expected 2 new containers (scale 2→4)")
	assert.Equal(t, recreates, 0, "expected 0 recreates with RecreateNever policy")
}

// TestReconcileForceRecreateNoDeps mirrors e2e TestRecreateWithNoDeps:
// --force-recreate with --no-deps on a single service should only recreate
// that service, not its dependencies.
func TestReconcileForceRecreateNoDeps(t *testing.T) {
	mySvc := types.ServiceConfig{
		Name:  "my-service",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{Condition: "service_started"},
		},
	}
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	myHash, err := ServiceHash(mySvc)
	assert.NilError(t, err)
	dbHash, err := ServiceHash(dbSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "recreate-no-deps",
		Services: types.Services{
			"my-service": mySvc,
			"db":         dbSvc,
		},
	}

	observed := &ObservedState{
		ProjectName: "recreate-no-deps",
		Containers: map[string]Containers{
			"my-service": {makeContainer("recreate-no-deps", "my-service", 1, myHash)},
			"db":         {makeContainer("recreate-no-deps", "db", 1, dbHash)},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	// Only target "my-service" with force recreate; deps get "never"
	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateNever,
		Services:             []string{"my-service"},
	})
	assert.NilError(t, err)

	myRecreates := countOps(plan, OpRecreateContainer, "my-service")
	dbRecreates := countOps(plan, OpRecreateContainer, "db")
	assert.Equal(t, myRecreates, 1, "expected 1 recreate for my-service (force)")
	assert.Equal(t, dbRecreates, 0, "expected 0 recreates for db (no-deps)")
}

// TestReconcileNetworkConfigChanged mirrors e2e TestNetworkConfigChanged:
// when a network's configuration changes (e.g., subnet), the plan should
// include a recreate-network operation.
func TestReconcileNetworkConfigChanged(t *testing.T) {
	originalNet := types.NetworkConfig{
		Name: "testproject_mynet",
		Ipam: types.IPAMConfig{
			Config: []*types.IPAMPool{{Subnet: "172.99.0.0/16"}},
		},
	}
	originalHash, err := NetworkHash(&originalNet)
	assert.NilError(t, err)

	// Now the desired config has a different subnet
	updatedNet := types.NetworkConfig{
		Name: "testproject_mynet",
		Ipam: types.IPAMConfig{
			Config: []*types.IPAMPool{{Subnet: "192.168.0.0/16"}},
		},
	}
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"test": types.ServiceConfig{Name: "test", Image: "nginx"},
		},
		Networks: types.Networks{
			"mynet": updatedNet,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks: map[string]ObservedNetwork{
			"mynet": {
				ID:         "net-old-id",
				Name:       "testproject_mynet",
				ConfigHash: originalHash,
			},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	found := false
	for _, op := range plan.Operations {
		if op.Type == OpRecreateNetwork {
			found = true
			assert.Equal(t, op.Resource, "testproject_mynet")
			assert.Equal(t, op.Reason, "config hash changed")
		}
	}
	assert.Assert(t, found, "expected OpRecreateNetwork when subnet changes")
}

// TestReconcileVolumeConfigChanged mirrors e2e TestUpRecreateVolumes:
// when a volume's config (e.g., labels) changes, the plan should include
// a recreate-volume operation.
func TestReconcileVolumeConfigChanged(t *testing.T) {
	originalVol := types.VolumeConfig{
		Name:   "testproject_my_vol",
		Labels: types.Labels{"foo": "bar"},
	}
	originalHash, err := VolumeHash(originalVol)
	assert.NilError(t, err)

	// Updated config with different label
	updatedVol := types.VolumeConfig{
		Name:   "testproject_my_vol",
		Labels: types.Labels{"foo": "zot"},
	}
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"app": types.ServiceConfig{Name: "app", Image: "nginx"},
		},
		Volumes: types.Volumes{
			"my_vol": updatedVol,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"my_vol": {
				Name:       "testproject_my_vol",
				Driver:     "local",
				ConfigHash: originalHash,
			},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	found := false
	for _, op := range plan.Operations {
		if op.Type == OpRecreateVolume {
			found = true
			assert.Equal(t, op.Resource, "testproject_my_vol")
			assert.Equal(t, op.Reason, "config hash changed")
		}
	}
	assert.Assert(t, found, "expected OpRecreateVolume when labels change")
}

// TestReconcileExternalNetworkSkipped verifies that external networks are
// never created or recreated, matching the behavior tested in e2e network tests.
func TestReconcileExternalNetworkSkipped(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Networks: types.Networks{
			"ext": types.NetworkConfig{Name: "external_net", External: true},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpCreateNetwork || op.Type == OpRecreateNetwork {
			t.Fatalf("external network should not be created/recreated: %s", op.ID)
		}
	}
}

// TestReconcileExternalVolumeSkipped verifies that external volumes are never
// created or recreated.
func TestReconcileExternalVolumeSkipped(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Volumes: types.Volumes{
			"ext": types.VolumeConfig{Name: "external_vol", External: true},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpCreateVolume || op.Type == OpRecreateVolume {
			t.Fatalf("external volume should not be created/recreated: %s", op.ID)
		}
	}
}

// TestReconcileOrphansNotRemovedByDefault mirrors e2e TestRemoveOrphans:
// orphan containers should NOT be removed unless RemoveOrphans is set.
func TestReconcileOrphansNotRemovedByDefault(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans: Containers{
			makeContainer("testproject", "old-service", 1, "some-hash"),
		},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        false,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Reason == "orphan container" {
			t.Fatalf("orphan should not be removed when RemoveOrphans=false: %s", op.ID)
		}
	}
}

// TestReconcileContainerCreateDependsOnNetworkAndVolume mirrors e2e
// TestUpWithAllResources: when a service uses a network and a volume,
// its create-container ops should depend on the network and volume create ops.
func TestReconcileContainerCreateDependsOnNetworkAndVolume(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"app": types.ServiceConfig{
				Name:  "app",
				Image: "nginx",
				Networks: map[string]*types.ServiceNetworkConfig{
					"mynet": {},
				},
				Volumes: []types.ServiceVolumeConfig{
					{Type: "volume", Source: "myvol", Target: "/data"},
				},
			},
		},
		Networks: types.Networks{
			"mynet": types.NetworkConfig{Name: "testproject_mynet"},
		},
		Volumes: types.Volumes{
			"myvol": types.VolumeConfig{Name: "testproject_myvol"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `Networks:
  create     testproject_mynet  "network does not exist"
Volumes:
  create     testproject_myvol  "volume does not exist"
Containers:
  create     testproject-app-1  "scale up"
    depends on: create-network:testproject_mynet, create-volume:testproject_myvol
`)
}

// TestReconcileImageDigestChanged mirrors the behavior tested in e2e
// volume/build tests where a container is recreated because the image
// digest has changed (e.g., after a rebuild).
func TestReconcileImageDigestChanged(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		CustomLabels: types.Labels{
			api.ImageDigestLabel: "sha256:newdigest",
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": svc,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "ctr1",
					Names: []string{"/testproject-web-1"},
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ImageDigestLabel:     "sha256:olddigest",
						api.ProjectLabel:         "testproject",
					},
					State: container.StateRunning,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	found := false
	for _, op := range plan.Operations {
		if op.Type == OpRecreateContainer && op.ServiceName == "web" {
			found = true
			assert.Equal(t, op.Reason, "image digest changed")
		}
	}
	assert.Assert(t, found, "expected OpRecreateContainer for image digest change")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func makeContainer(projectName, serviceName string, number int, configHash string) container.Summary {
	name := projectName + "-" + serviceName + "-" + fmt.Sprintf("%d", number)
	return container.Summary{
		ID:    fmt.Sprintf("%s-%s-%d", projectName, serviceName, number),
		Names: []string{"/" + name},
		Labels: map[string]string{
			api.ServiceLabel:         serviceName,
			api.ContainerNumberLabel: fmt.Sprintf("%d", number),
			api.ConfigHashLabel:      configHash,
			api.ProjectLabel:         projectName,
		},
		State: container.StateRunning,
	}
}

func countOps(plan *ReconciliationPlan, opType OperationType, serviceName string) int {
	count := 0
	for _, op := range plan.Operations {
		if op.Type == opType && op.ServiceName == serviceName {
			count++
		}
	}
	return count
}
