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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
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
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				Networks: map[string]*types.ServiceNetworkConfig{
					"default": nil,
				},
			},
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
	assert.Equal(t, plan.String(), `
1. create network testproject_default  reason: network does not exist
[1] -> 2. create container testproject-web-1  reason: scale up
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
		if op.Type == OpCreateNetwork || op.Type == OpRemoveNetwork {
			t.Fatalf("unexpected network operation: %s", op.ID)
		}
	}
}

func TestReconcileRecreateChangedNetwork(t *testing.T) {
	service := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	configHash, err := ServiceHash(service)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": service,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "testproject_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "ctr1",
					Names: []string{"/testproject-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      configHash,
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"testproject_default": {NetworkID: "net123"},
						},
					},
				},
			},
		},
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-web-1  reason: network "testproject_default" is being recreated
[1] -> 2. disconnect network testproject-web-1 from testproject_default  reason: network "testproject_default" is being recreated
[2] -> 3. remove network testproject_default  reason: config hash changed
[3] -> 4. create network testproject_default  reason: config hash changed
[4] -> 5. connect network testproject-web-1 to testproject_default  reason: network "testproject_default" has been recreated
[5] -> 6. start container testproject-web-1  reason: network "testproject_default" has been recreated
`)
}

func TestReconcileCreateMissingVolume(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				Volumes: []types.ServiceVolumeConfig{
					{Type: "volume", Source: "data", Target: "/data"},
				},
			},
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
	assert.Equal(t, plan.String(), `
1. create volume testproject_data  reason: volume does not exist
[1] -> 2. create container testproject-web-1  reason: scale up
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
	assert.Equal(t, plan.String(), `
1. create container testproject-web-2  reason: scale up
`)
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-web-2  reason: scale down
[1] -> 2. remove container testproject-web-2  reason: scale down
`)
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
					ID:    "abc123def456",
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
	assert.Equal(t, plan.String(), `
1. create container abc123def456_testproject-web-1  reason: config hash changed
[1] -> 2. stop container testproject-web-1  reason: config hash changed
[2] -> 3. remove container testproject-web-1  reason: config hash changed
[3] -> 4. rename container testproject-web-1  reason: config hash changed
[4] -> 5. start container testproject-web-1  reason: config hash changed
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
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
2. stop container testproject-old-1  reason: orphan container
[2] -> 3. remove container testproject-old-1  reason: orphan container
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
	assert.Equal(t, plan.String(), `
1. plugin plugin plugin-svc  reason: plugin service
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
	assert.Equal(t, plan.String(), `
1. create container testproject-db-1  reason: scale up
[1] -> 2. create container testproject-web-1  reason: scale up
`)
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
	assert.Equal(t, plan.String(), `
1. create container scale-basic-tests-back-2  reason: scale up
2. create container scale-basic-tests-front-2  reason: scale up
3. create container scale-basic-tests-front-3  reason: scale up
`)
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
	assert.Equal(t, plan.String(), `
1. stop container scale-basic-tests-back-2  reason: scale down
2. stop container scale-basic-tests-front-3  reason: scale down
[1] -> 3. remove container scale-basic-tests-back-2  reason: scale down
[2] -> 4. remove container scale-basic-tests-front-3  reason: scale down
`)
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-dbadmin-1  reason: scale down
2. stop container testproject-dbadmin-2  reason: scale down
[1] -> 3. remove container testproject-dbadmin-1  reason: scale down
[2] -> 4. remove container testproject-dbadmin-2  reason: scale down
`)
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
	// Obsolete container (db-1) is removed first, up-to-date one (db-2) stays
	assert.Equal(t, plan.String(), `
1. stop container testproject-db-1  reason: scale down
[1] -> 2. remove container testproject-db-1  reason: scale down
`)
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
	// Only new containers created, no recreates despite stale hash
	assert.Equal(t, plan.String(), `
1. create container testproject-test-3  reason: scale up
2. create container testproject-test-4  reason: scale up
`)
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
	// Only my-service is recreated, db is left untouched
	assert.Equal(t, plan.String(), `
1. create container recreate-no-_recreate-no-deps-my-service-1  reason: force recreate
[1] -> 2. stop container recreate-no-deps-my-service-1  reason: force recreate
[2] -> 3. remove container recreate-no-deps-my-service-1  reason: force recreate
[3] -> 4. rename container recreate-no-deps-my-service-1  reason: force recreate
[4] -> 5. start container recreate-no-deps-my-service-1  reason: force recreate
`)
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
	assert.Equal(t, plan.String(), `
1. create container testproject-test-1  reason: scale up
2. remove network testproject_mynet  reason: config hash changed
[2] -> 3. create network testproject_mynet  reason: config hash changed
`)
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
	assert.Equal(t, plan.String(), `
1. create container testproject-app-1  reason: scale up
2. remove volume testproject_my_vol  reason: config hash changed
[2] -> 3. create volume testproject_my_vol  reason: config hash changed
`)
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
	// External network is not created — only the container
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
`)
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
	// External volume is not created — only the container
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
`)
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
	// Orphan is ignored — only the web container is created
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
`)
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
	assert.Equal(t, plan.String(), `
1. create network testproject_mynet  reason: network does not exist
2. create volume testproject_myvol  reason: volume does not exist
[1,2] -> 3. create container testproject-app-1  reason: scale up
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
	assert.Equal(t, plan.String(), `
1. create container ctr1_testproject-web-1  reason: image digest changed
[1] -> 2. stop container testproject-web-1  reason: image digest changed
[2] -> 3. remove container testproject-web-1  reason: image digest changed
[3] -> 4. rename container testproject-web-1  reason: image digest changed
[4] -> 5. start container testproject-web-1  reason: image digest changed
`)
}

// ---------------------------------------------------------------------------
// 1. Stopped container gets started
// ---------------------------------------------------------------------------

func TestReconcileDeadContainerGetsStarted(t *testing.T) {
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
					State: container.StateDead,
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
	assert.Equal(t, plan.String(), `
1. start container testproject-web-1  reason: container not running (state: dead)
`)
}

// ---------------------------------------------------------------------------
// 2. Exited container is left alone
// ---------------------------------------------------------------------------

func TestReconcileExitedContainerNoOps(t *testing.T) {
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
					State: container.StateExited,
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

// ---------------------------------------------------------------------------
// 3. Force recreate on up-to-date container produces full chain
// ---------------------------------------------------------------------------

func TestReconcileForceRecreateUpToDate(t *testing.T) {
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
					ID:    "abc123def456",
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
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateForce,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container abc123def456_testproject-web-1  reason: force recreate
[1] -> 2. stop container testproject-web-1  reason: force recreate
[2] -> 3. remove container testproject-web-1  reason: force recreate
[3] -> 4. rename container testproject-web-1  reason: force recreate
[4] -> 5. start container testproject-web-1  reason: force recreate
`)
}

// ---------------------------------------------------------------------------
// 4. RecreateNever with stale containers — no ops
// ---------------------------------------------------------------------------

func TestReconcileNeverRecreateStaleContainers(t *testing.T) {
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
				makeContainer("testproject", "web", 1, "stale-hash"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateNever,
		RecreateDependencies: api.RecreateNever,
	})
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// ---------------------------------------------------------------------------
// 5. Network recreate with multiple connected containers
// ---------------------------------------------------------------------------

func TestReconcileNetworkRecreateMultipleContainers(t *testing.T) {
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	workerSvc := types.ServiceConfig{
		Name:  "worker",
		Image: "worker:latest",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	webHash, err := ServiceHash(webSvc)
	assert.NilError(t, err)
	workerHash, err := ServiceHash(workerSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web":    webSvc,
			"worker": workerSvc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "testproject_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "ctr-web",
					Names: []string{"/testproject-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      webHash,
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"testproject_default": {NetworkID: "net123"},
						},
					},
				},
			},
			"worker": {
				{
					ID:    "ctr-worker",
					Names: []string{"/testproject-worker-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "worker",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      workerHash,
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"testproject_default": {NetworkID: "net123"},
						},
					},
				},
			},
		},
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-web-1  reason: network "testproject_default" is being recreated
2. stop container testproject-worker-1  reason: network "testproject_default" is being recreated
[1] -> 3. disconnect network testproject-web-1 from testproject_default  reason: network "testproject_default" is being recreated
[2] -> 4. disconnect network testproject-worker-1 from testproject_default  reason: network "testproject_default" is being recreated
[3,4] -> 5. remove network testproject_default  reason: config hash changed
[5] -> 6. create network testproject_default  reason: config hash changed
[6] -> 7. connect network testproject-web-1 to testproject_default  reason: network "testproject_default" has been recreated
[6] -> 8. connect network testproject-worker-1 to testproject_default  reason: network "testproject_default" has been recreated
[7] -> 9. start container testproject-web-1  reason: network "testproject_default" has been recreated
[8] -> 10. start container testproject-worker-1  reason: network "testproject_default" has been recreated
`)
}

// ---------------------------------------------------------------------------
// 6. Container matched by network name (not ID)
// ---------------------------------------------------------------------------

func TestReconcileNetworkMatchByName(t *testing.T) {
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	webHash, err := ServiceHash(webSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": webSvc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "testproject_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "ctr-web",
					Names: []string{"/testproject-web-1"},
					// StateCreated avoids checkExpectedNetworks (only runs for running containers),
					// isolating the name-based matching in findContainersOnNetwork.
					State: container.StateCreated,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      webHash,
					},
					// NetworkID is empty — findContainersOnNetwork can only find this by name match
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"testproject_default": {NetworkID: ""},
						},
					},
				},
			},
		},
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-web-1  reason: network "testproject_default" is being recreated
[1] -> 2. disconnect network testproject-web-1 from testproject_default  reason: network "testproject_default" is being recreated
[2] -> 3. remove network testproject_default  reason: config hash changed
[3] -> 4. create network testproject_default  reason: config hash changed
[4] -> 5. connect network testproject-web-1 to testproject_default  reason: network "testproject_default" has been recreated
[5] -> 6. start container testproject-web-1  reason: network "testproject_default" has been recreated
`)
}

// ---------------------------------------------------------------------------
// 7. Service references network not in project.Networks
// ---------------------------------------------------------------------------

func TestReconcileServiceWithUnknownNetwork(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				Networks: map[string]*types.ServiceNetworkConfig{
					"nonexistent": nil,
				},
			},
		},
		Networks: types.Networks{}, // no networks defined
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
	// Should still produce a create container op without panicking
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
`)
}

// ---------------------------------------------------------------------------
// 8. Volume recreate with connected containers
// ---------------------------------------------------------------------------

func TestReconcileVolumeRecreateWithContainers(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "app",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "data", Target: "/data"},
		},
	}
	svcHash, err := ServiceHash(svc)
	assert.NilError(t, err)

	originalVol := types.VolumeConfig{Name: "testproject_data"}
	originalHash, err := VolumeHash(originalVol)
	assert.NilError(t, err)

	updatedVol := types.VolumeConfig{
		Name:   "testproject_data",
		Labels: types.Labels{"version": "2"},
	}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"app": svc,
		},
		Volumes: types.Volumes{
			"data": updatedVol,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"app": {
				{
					ID:    "ctr-app",
					Names: []string{"/testproject-app-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "app",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      svcHash,
					},
					Mounts: []container.MountPoint{
						{Type: "volume", Name: "testproject_data"},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {
				Name:       "testproject_data",
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
	assert.Equal(t, plan.String(), `
1. stop container testproject-app-1  reason: volume "testproject_data" is being recreated
[1] -> 2. remove container testproject-app-1  reason: volume "testproject_data" is being recreated
[2] -> 3. remove volume testproject_data  reason: config hash changed
[3] -> 4. create volume testproject_data  reason: config hash changed
[4] -> 5. create container testproject-app-1  reason: volume "data" is being recreated
`)
}

// ---------------------------------------------------------------------------
// 9. Bind mount does not trigger volume reconciliation
// ---------------------------------------------------------------------------

func TestReconcileBindMountNotAffectedByVolumeReconcile(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "app",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "bind", Source: "/host/path", Target: "/data"},
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"app": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"app": {
				makeContainerWithHash("testproject", "app", 1, hash),
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

// ---------------------------------------------------------------------------
// 10. Diamond dependency: D ← B,C ← A
// ---------------------------------------------------------------------------

func TestReconcileDiamondDependency(t *testing.T) {
	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"a": types.ServiceConfig{
				Name:  "a",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"b": types.ServiceDependency{Condition: "service_started"},
					"c": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"b": types.ServiceConfig{
				Name:  "b",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"d": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"c": types.ServiceConfig{
				Name:  "c",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"d": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"d": types.ServiceConfig{
				Name:  "d",
				Image: "nginx",
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
	assert.Equal(t, plan.String(), `
1. create container testproject-d-1  reason: scale up
[1] -> 2. create container testproject-b-1  reason: scale up
[1] -> 3. create container testproject-c-1  reason: scale up
[2,3] -> 4. create container testproject-a-1  reason: scale up
`)
}

// ---------------------------------------------------------------------------
// 11. Cascading restart when dependency is recreated (restart: true)
// ---------------------------------------------------------------------------

func TestReconcileCascadingRestart(t *testing.T) {
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{
				Condition: "service_started",
				Restart:   true,
			},
		},
	}
	webHash, err := ServiceHash(webSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"db": {
				makeContainer("testproject", "db", 1, "stale-hash"),
			},
			"web": {
				makeContainer("testproject", "web", 1, webHash),
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
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-db-1  reason: config hash changed
2. stop container testproject-web-1  reason: dependency "db" is being recreated (restart: true)
[1] -> 3. stop container testproject-db-1  reason: config hash changed
[3] -> 4. remove container testproject-db-1  reason: config hash changed
[4] -> 5. rename container testproject-db-1  reason: config hash changed
[5] -> 6. start container testproject-db-1  reason: config hash changed
[6,2] -> 7. start container testproject-web-1  reason: restart after dependency "db" recreated
`)
}

// ---------------------------------------------------------------------------
// 12. No cascading restart when restart: false
// ---------------------------------------------------------------------------

func TestReconcileNoCascadingRestartWhenFalse(t *testing.T) {
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{
				Condition: "service_started",
				Restart:   false,
			},
		},
	}
	webHash, err := ServiceHash(webSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"db": {
				makeContainer("testproject", "db", 1, "stale-hash"),
			},
			"web": {
				makeContainer("testproject", "web", 1, webHash),
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
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-db-1  reason: config hash changed
[1] -> 2. stop container testproject-db-1  reason: config hash changed
[2] -> 3. remove container testproject-db-1  reason: config hash changed
[3] -> 4. rename container testproject-db-1  reason: config hash changed
[4] -> 5. start container testproject-db-1  reason: config hash changed
`)
}

// ---------------------------------------------------------------------------
// 13. Scale up + config change simultaneously
// ---------------------------------------------------------------------------

func TestReconcileScaleUpWithConfigChange(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx", Scale: intPtr(3)}

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
				makeContainer("testproject", "web", 1, "stale-hash"),
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
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-web-1  reason: config hash changed
2. create container testproject-web-2  reason: scale up
3. create container testproject-web-3  reason: scale up
[1] -> 4. stop container testproject-web-1  reason: config hash changed
[4] -> 5. remove container testproject-web-1  reason: config hash changed
[5] -> 6. rename container testproject-web-1  reason: config hash changed
[6] -> 7. start container testproject-web-1  reason: config hash changed
`)
}

// ---------------------------------------------------------------------------
// 14. Scale down + config change
// ---------------------------------------------------------------------------

func TestReconcileScaleDownWithConfigChange(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}

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
				makeContainer("testproject", "web", 1, "stale-hash-1"),
				makeContainer("testproject", "web", 2, "stale-hash-2"),
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
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-web-1  reason: config hash changed
2. stop container testproject-web-2  reason: scale down
[1] -> 3. stop container testproject-web-1  reason: config hash changed
[2] -> 4. remove container testproject-web-2  reason: scale down
[3] -> 5. remove container testproject-web-1  reason: config hash changed
[5] -> 6. rename container testproject-web-1  reason: config hash changed
[6] -> 7. start container testproject-web-1  reason: config hash changed
`)
}

// ---------------------------------------------------------------------------
// 15. Custom container_name with scale > 1 returns error
// ---------------------------------------------------------------------------

func TestReconcileCustomContainerNameScaleError(t *testing.T) {
	svc := types.ServiceConfig{
		Name:          "web",
		Image:         "nginx",
		Scale:         intPtr(2),
		ContainerName: "my-custom-name",
	}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	_, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.Assert(t, err != nil, "expected error for custom container_name with scale > 1")
}

// ---------------------------------------------------------------------------
// 16. Targeted service with dependency — dep uses RecreateDependencies policy
// ---------------------------------------------------------------------------

func TestReconcileTargetedServiceDependencyPolicy(t *testing.T) {
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{Condition: "service_started"},
		},
	}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}

	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"db": {
				makeContainer("testproject", "db", 1, "stale-hash"),
			},
			"web": {
				makeContainer("testproject", "web", 1, "stale-hash"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	// Target only "web" with force-recreate; deps get "never" — db is NOT recreated
	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateNever,
		Services:             []string{"web"},
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-web-1  reason: force recreate
[1] -> 2. stop container testproject-web-1  reason: force recreate
[2] -> 3. remove container testproject-web-1  reason: force recreate
[3] -> 4. rename container testproject-web-1  reason: force recreate
[4] -> 5. start container testproject-web-1  reason: force recreate
`)

	// Same setup but deps get "diverged" — db IS stale so it gets recreated too
	plan2, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateDiverged,
		Services:             []string{"web"},
	})
	assert.NilError(t, err)
	assert.Equal(t, plan2.String(), `
1. create container testproject-_testproject-db-1  reason: config hash changed
[1] -> 2. stop container testproject-db-1  reason: config hash changed
[2] -> 3. remove container testproject-db-1  reason: config hash changed
[3] -> 4. rename container testproject-db-1  reason: config hash changed
[4] -> 5. start container testproject-db-1  reason: config hash changed
[5] -> 6. create container testproject-_testproject-web-1  reason: force recreate
[6] -> 7. stop container testproject-web-1  reason: force recreate
[7] -> 8. remove container testproject-web-1  reason: force recreate
[8] -> 9. rename container testproject-web-1  reason: force recreate
[9,5] -> 10. start container testproject-web-1  reason: force recreate
`)
}

// ---------------------------------------------------------------------------
// 17. Non-targeted service is skipped entirely
// ---------------------------------------------------------------------------

func TestReconcileNonTargetedServiceSkipped(t *testing.T) {
	webSvc := types.ServiceConfig{Name: "web", Image: "nginx"}
	workerSvc := types.ServiceConfig{Name: "worker", Image: "worker:latest"}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web":    webSvc,
			"worker": workerSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers: map[string]Containers{
			"web": {
				makeContainer("testproject", "web", 1, "stale-hash"),
			},
			"worker": {
				makeContainer("testproject", "worker", 1, "stale-hash"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		Services:             []string{"web"},
	})
	assert.NilError(t, err)
	// Only web is recreated — worker is completely skipped
	assert.Equal(t, plan.String(), `
1. create container testproject-_testproject-web-1  reason: config hash changed
[1] -> 2. stop container testproject-web-1  reason: config hash changed
[2] -> 3. remove container testproject-web-1  reason: config hash changed
[3] -> 4. rename container testproject-web-1  reason: config hash changed
[4] -> 5. start container testproject-web-1  reason: config hash changed
`)
}

// ---------------------------------------------------------------------------
// 18. Empty project produces empty plan
// ---------------------------------------------------------------------------

func TestReconcileEmptyProject(t *testing.T) {
	project := &types.Project{
		Name:     "testproject",
		Services: types.Services{},
		Networks: types.Networks{},
		Volumes:  types.Volumes{},
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
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// ---------------------------------------------------------------------------
// 19. Multiple orphans with RemoveOrphans: true
// ---------------------------------------------------------------------------

func TestReconcileMultipleOrphans(t *testing.T) {
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
			makeContainer("testproject", "old-a", 1, "hash-a"),
			makeContainer("testproject", "old-b", 1, "hash-b"),
			makeContainer("testproject", "old-c", 1, "hash-c"),
		},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container testproject-web-1  reason: scale up
2. stop container testproject-old-a-1  reason: orphan container
3. stop container testproject-old-b-1  reason: orphan container
4. stop container testproject-old-c-1  reason: orphan container
[2] -> 5. remove container testproject-old-a-1  reason: orphan container
[3] -> 6. remove container testproject-old-b-1  reason: orphan container
[4] -> 7. remove container testproject-old-c-1  reason: orphan container
`)
}

// ---------------------------------------------------------------------------
// 20. Plugin service unaffected by recreate policies
// ---------------------------------------------------------------------------

func TestReconcilePluginServiceIgnoresRecreatePolicy(t *testing.T) {
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

	// Test with RecreateNever — plugin should still get an op
	plan, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateNever,
		RecreateDependencies: api.RecreateNever,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. plugin plugin plugin-svc  reason: plugin service
`)

	// Test with RecreateForce — same result
	plan2, err := Reconcile(project, observed, ReconcileOptions{
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateForce,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan2.String(), `
1. plugin plugin plugin-svc  reason: plugin service
`)
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

// makeContainerWithHash is like makeContainer but returns a container
// with the given hash precomputed (alias for readability).
func makeContainerWithHash(projectName, serviceName string, number int, configHash string) container.Summary {
	return makeContainer(projectName, serviceName, number, configHash)
}
