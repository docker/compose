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
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	mmount "github.com/moby/moby/api/types/mount"
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container testproject-old-1  reason: orphan container
[1] -> 2. remove container testproject-old-1  reason: orphan container
[2] -> 3. create container testproject-web-1  reason: scale up
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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

// ---------------------------------------------------------------------------
// 5b. Container connected to TWO recreated networks — start must depend on both connects
// ---------------------------------------------------------------------------

func TestReconcileMultiNetworkContainerReconnectDeps(t *testing.T) {
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"frontend": nil,
			"backend":  nil,
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
			"backend":  types.NetworkConfig{Name: "testproject_backend"},
			"frontend": types.NetworkConfig{Name: "testproject_frontend"},
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
							"testproject_frontend": {NetworkID: "net-fe"},
							"testproject_backend":  {NetworkID: "net-be"},
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"frontend": {ID: "net-fe", Name: "testproject_frontend", ConfigHash: "old-hash-fe"},
			"backend":  {ID: "net-be", Name: "testproject_backend", ConfigHash: "old-hash-be"},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// The start-container op must depend on BOTH connect ops.
	startOp, exists := plan.Operations["start-container:testproject-web-1"]
	assert.Assert(t, exists, "start-container op must exist")
	assert.Assert(t, len(startOp.DependsOn) == 2, "start must depend on both connect ops, got %v", startOp.DependsOn)

	// Both connect ops must be listed
	connectFe := "connect-network:testproject_frontend/testproject-web-1"
	connectBe := "connect-network:testproject_backend/testproject-web-1"
	for _, dep := range []string{connectFe, connectBe} {
		found := false
		for _, d := range startOp.DependsOn {
			if d == dep {
				found = true
				break
			}
		}
		assert.Assert(t, found, "start-container should depend on %s", dep)
	}
}

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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
[4] -> 6. stop container testproject-web-1  reason: dependency "db" is being recreated (restart: true)
[5,6] -> 7. start container testproject-web-1  reason: restart after dependency "db" recreated
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
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
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container testproject-old-a-1  reason: orphan container
2. stop container testproject-old-b-1  reason: orphan container
3. stop container testproject-old-c-1  reason: orphan container
[1] -> 4. remove container testproject-old-a-1  reason: orphan container
[2] -> 5. remove container testproject-old-b-1  reason: orphan container
[3] -> 6. remove container testproject-old-c-1  reason: orphan container
[4,5,6] -> 7. create container testproject-web-1  reason: scale up
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
		StartContainers:      true,
		Recreate:             api.RecreateNever,
		RecreateDependencies: api.RecreateNever,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. plugin plugin plugin-svc  reason: plugin service
`)

	// Test with RecreateForce — same result
	plan2, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateForce,
		RecreateDependencies: api.RecreateForce,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan2.String(), `
1. plugin plugin plugin-svc  reason: plugin service
`)
}

// ---------------------------------------------------------------------------
// Corner-case tests
// ---------------------------------------------------------------------------

// TestReconcileNonContiguousScaleDown verifies that when scaling down with
// non-sequential container numbers (gaps), the highest numbers are removed first.
func TestReconcileNonContiguousScaleDown(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				makeContainer("tp", "web", 1, hash),
				makeContainer("tp", "web", 3, hash),
				makeContainer("tp", "web", 5, hash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container tp-web-3  reason: scale down
2. stop container tp-web-5  reason: scale down
[1] -> 3. remove container tp-web-3  reason: scale down
[2] -> 4. remove container tp-web-5  reason: scale down
`)
}

// TestReconcileScaleUpFillsAfterMax verifies that new containers are numbered
// after the current maximum, not filling gaps.
func TestReconcileScaleUpFillsAfterMax(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx", Scale: intPtr(3)}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				makeContainer("tp", "web", 1, hash),
				makeContainer("tp", "web", 3, hash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// New container numbered 4 (max+1), not 2 (the gap)
	assert.Equal(t, plan.String(), `
1. create container tp-web-4  reason: scale up
`)
}

// TestReconcileInvalidContainerNumberFallback verifies that containers with
// invalid ContainerNumberLabel fall back to Created-timestamp-based sorting.
func TestReconcileInvalidContainerNumberFallback(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "c1",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "abc", // invalid
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					Created: 100,
				},
				{
					ID:    "c2",
					Names: []string{"/tp-web-2"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "2",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					Created: 200,
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Container with invalid number (older Created) is removed via scale down
	assert.Equal(t, plan.String(), `
1. stop container tp-web-1  reason: scale down
[1] -> 2. remove container tp-web-1  reason: scale down
`)
}

// TestReconcilePausedContainerGetsStarted verifies that paused containers
// trigger a start operation (paused is NOT in the "no action" list).
func TestReconcilePausedContainerGetsStarted(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	ctr := makeContainer("tp", "web", 1, hash)
	ctr.State = container.StatePaused

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{"web": {ctr}},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. start container tp-web-1  reason: container not running (state: paused)
`)
}

// TestReconcileRestartingContainerNoOps verifies that a restarting container
// with matching config produces no operations.
func TestReconcileRestartingContainerNoOps(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	ctr := makeContainer("tp", "web", 1, hash)
	ctr.State = container.StateRestarting

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{"web": {ctr}},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// TestReconcileNetworkCheckSkippedNonRunning verifies that checkExpectedNetworks
// is NOT called for non-running containers (only gated on StateRunning).
func TestReconcileNetworkCheckSkippedNonRunning(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"mynet": nil,
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	netCfg := types.NetworkConfig{Name: "tp_mynet"}
	netHash, err := NetworkHash(&netCfg)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"mynet": netCfg,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "c1",
					Names: []string{"/tp-web-1"},
					// StateCreated → checkExpectedNetworks skipped
					State: container.StateCreated,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					// Connected to a different network ID — would trigger recreate if running
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"tp_mynet": {NetworkID: "wrong-id"},
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"mynet": {
				ID:         "correct-id",
				Name:       "tp_mynet",
				ConfigHash: netHash,
			},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// No recreate despite network mismatch — container is not running
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// TestReconcileSwarmNetworkSkipped verifies that the "swarm" overlay network
// special case is handled correctly — containers using it should not trigger
// a false "network configuration changed" recreate.
func TestReconcileSwarmNetworkSkipped(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"overlay": nil,
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	netCfg := types.NetworkConfig{Name: "tp_overlay"}
	netHash, err := NetworkHash(&netCfg)
	assert.NilError(t, err)

	ctr := makeContainer("tp", "web", 1, hash)
	ctr.NetworkSettings = &container.NetworkSettingsSummary{
		Networks: map[string]*network.EndpointSettings{
			"tp_overlay": {NetworkID: "net1"},
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"overlay": netCfg,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{"web": {ctr}},
		Networks: map[string]ObservedNetwork{
			// Network ID is "swarm" — the special case
			"overlay": {ID: "swarm", Name: "tp_overlay", ConfigHash: netHash},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// TestReconcileNilNetworkSettingsNoPanic verifies that a container with nil
// NetworkSettings does not cause a panic during network recreate.
func TestReconcileNilNetworkSettingsNoPanic(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "tp_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "c1",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					NetworkSettings: nil, // nil — must not panic
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"default": {ID: "net1", Name: "tp_default", ConfigHash: "outdated"},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Container is skipped by findContainersOnNetwork (nil settings),
	// network is still recreated
	assert.Equal(t, plan.String(), `
1. remove network tp_default  reason: config hash changed
[1] -> 2. create network tp_default  reason: config hash changed
`)
}

// TestReconcileExternalNetworkResolvedFromContainer verifies that external
// network IDs are resolved from running containers' network endpoints,
// preventing a false "network configuration changed" recreate.
func TestReconcileExternalNetworkResolvedFromContainer(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"ext": nil,
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	ctr := makeContainer("tp", "web", 1, hash)
	ctr.NetworkSettings = &container.NetworkSettingsSummary{
		Networks: map[string]*network.EndpointSettings{
			"shared_net": {NetworkID: "extnet123"},
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"ext": types.NetworkConfig{Name: "shared_net", External: true},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{"web": {ctr}},
		Networks:    map[string]ObservedNetwork{}, // external net not in observed
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// No recreate — external network ID resolved from container
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// TestReconcileAnonymousVolumeNoOps verifies that anonymous volumes
// (empty Source) do not interfere with volume reconciliation.
func TestReconcileAnonymousVolumeNoOps(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "", Target: "/data"},
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {makeContainer("tp", "web", 1, hash)},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// TestReconcileVolumeRecreateUnrelatedServiceUnaffected verifies that when a
// volume is recreated, only services that mount it are affected.
func TestReconcileVolumeRecreateUnrelatedServiceUnaffected(t *testing.T) {
	appSvc := types.ServiceConfig{
		Name:  "app",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "data", Target: "/data"},
		},
	}
	workerSvc := types.ServiceConfig{Name: "worker", Image: "worker:latest"}
	appHash, err := ServiceHash(appSvc)
	assert.NilError(t, err)
	workerHash, err := ServiceHash(workerSvc)
	assert.NilError(t, err)

	origVol := types.VolumeConfig{Name: "tp_data"}
	origHash, err := VolumeHash(origVol)
	assert.NilError(t, err)

	updatedVol := types.VolumeConfig{
		Name:   "tp_data",
		Labels: types.Labels{"v": "2"},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"app":    appSvc,
			"worker": workerSvc,
		},
		Volumes: types.Volumes{
			"data": updatedVol,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"app": {
				{
					ID:    "ctr-app",
					Names: []string{"/tp-app-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "app",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "tp",
						api.ConfigHashLabel:      appHash,
					},
					Mounts: []container.MountPoint{
						{Type: mmount.TypeVolume, Name: "tp_data"},
					},
				},
			},
			"worker": {makeContainer("tp", "worker", 1, workerHash)},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "tp_data", Driver: "local", ConfigHash: origHash},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Only app is affected — worker has no volume ops
	assert.Equal(t, plan.String(), `
1. stop container tp-app-1  reason: volume "tp_data" is being recreated
[1] -> 2. remove container tp-app-1  reason: volume "tp_data" is being recreated
[2] -> 3. remove volume tp_data  reason: config hash changed
[3] -> 4. create volume tp_data  reason: config hash changed
[4] -> 5. create container tp-app-1  reason: volume "data" is being recreated
`)
}

// TestReconcileCircularDependencyNoPanic verifies that circular service
// dependencies do not cause infinite recursion in expandServiceDependencies.
func TestReconcileCircularDependencyNoPanic(t *testing.T) {
	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"a": types.ServiceConfig{
				Name:  "a",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"b": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"b": types.ServiceConfig{
				Name:  "b",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"a": types.ServiceDependency{Condition: "service_started"},
				},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	// Should not panic — expandServiceDependencies uses a seen map
	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, !plan.IsEmpty(), "expected create ops for both services")
}

// TestReconcileCascadingRestartMultipleDepsOneRecreated verifies cascading
// restart fires when only one of multiple dependencies is recreated.
func TestReconcileCascadingRestartMultipleDepsOneRecreated(t *testing.T) {
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	cacheSvc := types.ServiceConfig{Name: "cache", Image: "redis"}
	cacheHash, err := ServiceHash(cacheSvc)
	assert.NilError(t, err)
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db":    types.ServiceDependency{Condition: "service_started", Restart: true},
			"cache": types.ServiceDependency{Condition: "service_started", Restart: true},
		},
	}
	webHash, err := ServiceHash(webSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db":    dbSvc,
			"cache": cacheSvc,
			"web":   webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"db":    {makeContainer("tp", "db", 1, "stale")},      // stale → recreated
			"cache": {makeContainer("tp", "cache", 1, cacheHash)}, // up-to-date
			"web":   {makeContainer("tp", "web", 1, webHash)},     // up-to-date
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-db-1_tp-db-1  reason: config hash changed
[1] -> 2. stop container tp-db-1  reason: config hash changed
[2] -> 3. remove container tp-db-1  reason: config hash changed
[3] -> 4. rename container tp-db-1  reason: config hash changed
[4] -> 5. start container tp-db-1  reason: config hash changed
[4] -> 6. stop container tp-web-1  reason: dependency "db" is being recreated (restart: true)
[5,6] -> 7. start container tp-web-1  reason: restart after dependency "db" recreated
`)
}

// TestReconcileCascadingRestartSkippedForExitedDependent verifies that
// cascading restart is skipped when the dependent container is not running.
func TestReconcileCascadingRestartSkippedForExitedDependent(t *testing.T) {
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

	webCtr := makeContainer("tp", "web", 1, webHash)
	webCtr.State = container.StateExited

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"db":  {makeContainer("tp", "db", 1, "stale")},
			"web": {webCtr},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Only db is recreated — web is exited, no cascading restart
	assert.Equal(t, plan.String(), `
1. create container tp-db-1_tp-db-1  reason: config hash changed
[1] -> 2. stop container tp-db-1  reason: config hash changed
[2] -> 3. remove container tp-db-1  reason: config hash changed
[3] -> 4. rename container tp-db-1  reason: config hash changed
[4] -> 5. start container tp-db-1  reason: config hash changed
`)
}

// TestReconcileCustomContainerNameScale1Allowed verifies that container_name
// with the default scale (1) works without error.
func TestReconcileCustomContainerNameScale1Allowed(t *testing.T) {
	svc := types.ServiceConfig{
		Name:          "web",
		Image:         "nginx",
		ContainerName: "my-app",
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container my-app  reason: scale up
`)
}

// TestReconcileCustomContainerNameScale0Allowed verifies that container_name
// with scale=0 works without error and scales down existing containers.
func TestReconcileCustomContainerNameScale0Allowed(t *testing.T) {
	svc := types.ServiceConfig{
		Name:          "web",
		Image:         "nginx",
		ContainerName: "my-app",
		Scale:         intPtr(0),
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "c1",
					Names: []string{"/my-app"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container my-app  reason: scale down
[1] -> 2. remove container my-app  reason: scale down
`)
}

// TestReconcileShortContainerIDInRecreate verifies that container IDs shorter
// than 12 characters don't cause a slice bounds panic in temp name generation.
func TestReconcileShortContainerIDInRecreate(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123", // 6 chars — shorter than 12
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      "stale",
						api.ProjectLabel:         "tp",
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container abc123_tp-web-1  reason: config hash changed
[1] -> 2. stop container tp-web-1  reason: config hash changed
[2] -> 3. remove container tp-web-1  reason: config hash changed
[3] -> 4. rename container tp-web-1  reason: config hash changed
[4] -> 5. start container tp-web-1  reason: config hash changed
`)
}

// TestReconcileTwoServicesShareRecreatedVolume verifies that when two services
// mount the same volume and it's recreated, both get stop+remove+create ops.
func TestReconcileTwoServicesShareRecreatedVolume(t *testing.T) {
	appSvc := types.ServiceConfig{
		Name:  "app",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "shared", Target: "/data"},
		},
	}
	workerSvc := types.ServiceConfig{
		Name:  "worker",
		Image: "worker:latest",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "shared", Target: "/data"},
		},
	}
	appHash, err := ServiceHash(appSvc)
	assert.NilError(t, err)
	workerHash, err := ServiceHash(workerSvc)
	assert.NilError(t, err)

	origVol := types.VolumeConfig{Name: "tp_shared"}
	origHash, err := VolumeHash(origVol)
	assert.NilError(t, err)

	updatedVol := types.VolumeConfig{
		Name:   "tp_shared",
		Labels: types.Labels{"v": "2"},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"app":    appSvc,
			"worker": workerSvc,
		},
		Volumes: types.Volumes{
			"shared": updatedVol,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"app": {
				{
					ID:    "ctr-app",
					Names: []string{"/tp-app-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "app",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "tp",
						api.ConfigHashLabel:      appHash,
					},
					Mounts: []container.MountPoint{
						{Type: mmount.TypeVolume, Name: "tp_shared"},
					},
				},
			},
			"worker": {
				{
					ID:    "ctr-worker",
					Names: []string{"/tp-worker-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "worker",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "tp",
						api.ConfigHashLabel:      workerHash,
					},
					Mounts: []container.MountPoint{
						{Type: mmount.TypeVolume, Name: "tp_shared"},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"shared": {Name: "tp_shared", Driver: "local", ConfigHash: origHash},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container tp-app-1  reason: volume "tp_shared" is being recreated
2. stop container tp-worker-1  reason: volume "tp_shared" is being recreated
[1] -> 3. remove container tp-app-1  reason: volume "tp_shared" is being recreated
[2] -> 4. remove container tp-worker-1  reason: volume "tp_shared" is being recreated
[3,4] -> 5. remove volume tp_shared  reason: config hash changed
[5] -> 6. create volume tp_shared  reason: config hash changed
[6] -> 7. create container tp-app-1  reason: volume "shared" is being recreated
[6] -> 8. create container tp-worker-1  reason: volume "shared" is being recreated
`)
}

// TestReconcileDependsOnMissingServiceNoPanic verifies that a depends_on
// reference to a non-existent service doesn't panic — it's silently ignored.
func TestReconcileDependsOnMissingServiceNoPanic(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"ghost": types.ServiceDependency{Condition: "service_started"},
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// "ghost" is silently ignored — web is created without dependency edge
	assert.Equal(t, plan.String(), `
1. create container tp-web-1  reason: scale up
`)
}

// TestReconcileStaleConfigAndNetworkRecreate verifies that when a container
// has both a stale config hash AND is connected to a network being recreated,
// the plan contains valid operations for both paths without panicking.
func TestReconcileStaleConfigAndNetworkRecreate(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "tp_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123def456",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      "stale",
						api.ProjectLabel:         "tp",
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"tp_default": {NetworkID: "net1"},
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"default": {ID: "net1", Name: "tp_default", ConfigHash: "outdated"},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Both network recreate ops and container recreate ops coexist — plan is non-empty
	assert.Assert(t, !plan.IsEmpty(), "expected non-empty plan")
	// The plan has ops from both paths (network: stop/disconnect/remove/create/connect/start
	// and container: create-temp/stop/remove/rename/start). Some ops share IDs, so the
	// network stop overwrites the container stop. Verify key ops exist.
	var hasNetworkRemove, hasContainerCreate bool
	for _, op := range plan.Operations {
		if op.Type == OpRemoveNetwork {
			hasNetworkRemove = true
		}
		if op.Type == OpCreateContainer {
			hasContainerCreate = true
		}
	}
	assert.Assert(t, hasNetworkRemove, "expected network remove op")
	assert.Assert(t, hasContainerCreate, "expected container create op")
}

// ---------------------------------------------------------------------------
// Volume mount mismatch tests
// ---------------------------------------------------------------------------

// TestReconcileVolumeMountMissingTriggersRecreate verifies that when a container's
// config hash matches but it's missing a volume mount, checkExpectedVolumes triggers
// a "volume configuration changed" recreate.
func TestReconcileVolumeMountMissingTriggersRecreate(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "data", Target: "/data"},
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	volCfg := types.VolumeConfig{Name: "tp_data"}
	volHash, err := VolumeHash(volCfg)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Volumes: types.Volumes{
			"data": volCfg,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123def456",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					Mounts: []container.MountPoint{}, // no mounts
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "tp_data", Driver: "local", ConfigHash: volHash},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container abc123def456_tp-web-1  reason: volume configuration changed
[1] -> 2. stop container tp-web-1  reason: volume configuration changed
[2] -> 3. remove container tp-web-1  reason: volume configuration changed
[3] -> 4. rename container tp-web-1  reason: volume configuration changed
[4] -> 5. start container tp-web-1  reason: volume configuration changed
`)
}

// TestReconcileMultipleVolumesOneMissing verifies that when a container has
// two declared volumes but only one is mounted, it triggers recreate.
func TestReconcileMultipleVolumesOneMissing(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "data", Target: "/data"},
			{Type: "volume", Source: "logs", Target: "/logs"},
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	volData := types.VolumeConfig{Name: "tp_data"}
	volLogs := types.VolumeConfig{Name: "tp_logs"}
	volDataHash, err := VolumeHash(volData)
	assert.NilError(t, err)
	volLogsHash, err := VolumeHash(volLogs)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Volumes: types.Volumes{
			"data": volData,
			"logs": volLogs,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "abc123def456",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					// only data mounted, logs missing
					Mounts: []container.MountPoint{
						{Type: mmount.TypeVolume, Name: "tp_data"},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {Name: "tp_data", Driver: "local", ConfigHash: volDataHash},
			"logs": {Name: "tp_logs", Driver: "local", ConfigHash: volLogsHash},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container abc123def456_tp-web-1  reason: volume configuration changed
[1] -> 2. stop container tp-web-1  reason: volume configuration changed
[2] -> 3. remove container tp-web-1  reason: volume configuration changed
[3] -> 4. rename container tp-web-1  reason: volume configuration changed
[4] -> 5. start container tp-web-1  reason: volume configuration changed
`)
}

// ---------------------------------------------------------------------------
// Timeout propagation
// ---------------------------------------------------------------------------

// TestReconcileTimeoutPropagated verifies that the Timeout option is carried
// through to stop operations in scale-down.
func TestReconcileTimeoutPropagated(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	timeout := 30 * time.Second

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				makeContainer("tp", "web", 1, hash),
				makeContainer("tp", "web", 2, hash),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		Timeout:              &timeout,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container tp-web-2  reason: scale down
[1] -> 2. remove container tp-web-2  reason: scale down
`)
	// Verify timeout is set on the stop op
	stopOp := plan.Operations["stop-container:tp-web-2"]
	assert.Assert(t, stopOp.ContainerOp.Timeout != nil)
	assert.Equal(t, *stopOp.ContainerOp.Timeout, timeout)
}

// ---------------------------------------------------------------------------
// Scale edge cases
// ---------------------------------------------------------------------------

// TestReconcileScaleUpFromZeroContainers verifies that scaling up when
// there are zero existing containers (first deploy) works correctly.
func TestReconcileScaleUpFromZeroContainers(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx", Scale: intPtr(2)}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{}, // nil entry for "web"
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-web-1  reason: scale up
2. create container tp-web-2  reason: scale up
`)
}

// TestReconcileAllContainersObsolete verifies that when all containers have
// stale hashes and scale is unchanged, each gets a full recreate chain.
func TestReconcileAllContainersObsolete(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx", Scale: intPtr(3)}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				makeContainer("tp", "web", 1, "stale1"),
				makeContainer("tp", "web", 2, "stale2"),
				makeContainer("tp", "web", 3, "stale3"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-web-1_tp-web-1  reason: config hash changed
2. create container tp-web-2_tp-web-2  reason: config hash changed
3. create container tp-web-3_tp-web-3  reason: config hash changed
[1] -> 4. stop container tp-web-1  reason: config hash changed
[2] -> 5. stop container tp-web-2  reason: config hash changed
[3] -> 6. stop container tp-web-3  reason: config hash changed
[4] -> 7. remove container tp-web-1  reason: config hash changed
[5] -> 8. remove container tp-web-2  reason: config hash changed
[6] -> 9. remove container tp-web-3  reason: config hash changed
[7] -> 10. rename container tp-web-1  reason: config hash changed
[8] -> 11. rename container tp-web-2  reason: config hash changed
[9] -> 12. rename container tp-web-3  reason: config hash changed
[10] -> 13. start container tp-web-1  reason: config hash changed
[11] -> 14. start container tp-web-2  reason: config hash changed
[12] -> 15. start container tp-web-3  reason: config hash changed
`)
}

// TestReconcileScaleDownStaleRemovedCurrentKept verifies that when scaling
// down with a mix of stale and current containers, the stale ones are removed
// and the current one is kept.
func TestReconcileScaleDownStaleRemovedCurrentKept(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				makeContainer("tp", "web", 1, "stale1"),
				makeContainer("tp", "web", 2, hash), // current
				makeContainer("tp", "web", 3, "stale3"),
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// Both stale containers removed, current container #2 survives
	assert.Equal(t, plan.String(), `
1. stop container tp-web-1  reason: scale down
2. stop container tp-web-3  reason: scale down
[1] -> 3. remove container tp-web-1  reason: scale down
[2] -> 4. remove container tp-web-3  reason: scale down
`)
}

// ---------------------------------------------------------------------------
// Dependency edge wiring
// ---------------------------------------------------------------------------

// TestReconcileRecreateNoEdgeToRunningDependency verifies that when a service
// is recreated but its dependency is already running and up-to-date, no
// dependency edge is added to the recreate chain.
func TestReconcileRecreateNoEdgeToRunningDependency(t *testing.T) {
	dbSvc := types.ServiceConfig{Name: "db", Image: "postgres"}
	dbHash, err := ServiceHash(dbSvc)
	assert.NilError(t, err)
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{Condition: "service_started"},
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"db":  {makeContainer("tp", "db", 1, dbHash)}, // up-to-date
			"web": {makeContainer("tp", "web", 1, "stale")},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// No dependency edge on db — it has no "ready" op in the plan
	assert.Equal(t, plan.String(), `
1. create container tp-web-1_tp-web-1  reason: config hash changed
[1] -> 2. stop container tp-web-1  reason: config hash changed
[2] -> 3. remove container tp-web-1  reason: config hash changed
[3] -> 4. rename container tp-web-1  reason: config hash changed
[4] -> 5. start container tp-web-1  reason: config hash changed
`)
}

// TestReconcileTwoServicesDependOnSameService verifies that when two services
// depend on the same missing service, both get dependency edges to it.
func TestReconcileTwoServicesDependOnSameService(t *testing.T) {
	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db": types.ServiceConfig{Name: "db", Image: "postgres"},
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				DependsOn: types.DependsOnConfig{
					"db": types.ServiceDependency{Condition: "service_started"},
				},
			},
			"worker": types.ServiceConfig{
				Name:  "worker",
				Image: "worker",
				DependsOn: types.DependsOnConfig{
					"db": types.ServiceDependency{Condition: "service_started"},
				},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-db-1  reason: scale up
[1] -> 2. create container tp-web-1  reason: scale up
[1] -> 3. create container tp-worker-1  reason: scale up
`)
}

// TestReconcileContainerCreateDependsOnRecreatedNetwork verifies that when a
// network is being recreated (not just created), a new container using that
// network depends on the network create op.
func TestReconcileContainerCreateDependsOnRecreatedNetwork(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "tp_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{}, // no containers yet
		Networks: map[string]ObservedNetwork{
			"default": {ID: "net1", Name: "tp_default", ConfigHash: "outdated"},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. remove network tp_default  reason: config hash changed
[1] -> 2. create network tp_default  reason: config hash changed
[2] -> 3. create container tp-web-1  reason: scale up
`)
}

// ---------------------------------------------------------------------------
// Cascading restart edge case
// ---------------------------------------------------------------------------

// TestReconcileCascadingRestartSkippedWhenAlreadyRecreating verifies that
// when a service is already being recreated (stale hash), cascading restart
// does not add duplicate stop/start ops.
func TestReconcileCascadingRestartSkippedWhenAlreadyRecreating(t *testing.T) {
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

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db":  dbSvc,
			"web": webSvc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"db":  {makeContainer("tp", "db", 1, "stale-db")},
			"web": {makeContainer("tp", "web", 1, "stale-web")}, // also stale
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// web is already being recreated — cascading restart does NOT add a duplicate stop
	assert.Equal(t, plan.String(), `
1. create container tp-db-1_tp-db-1  reason: config hash changed
[1] -> 2. stop container tp-db-1  reason: config hash changed
[2] -> 3. remove container tp-db-1  reason: config hash changed
[3] -> 4. rename container tp-db-1  reason: config hash changed
[4] -> 5. start container tp-db-1  reason: config hash changed
[5] -> 6. create container tp-web-1_tp-web-1  reason: config hash changed
[6] -> 7. stop container tp-web-1  reason: config hash changed
[7] -> 8. remove container tp-web-1  reason: config hash changed
[8] -> 9. rename container tp-web-1  reason: config hash changed
[9,5] -> 10. start container tp-web-1  reason: config hash changed
`)
	// Verify exactly one stop op for web
	var stopCount int
	for _, op := range plan.Operations {
		if op.Type == OpStopContainer && op.Resource == "tp-web-1" {
			stopCount++
		}
	}
	assert.Equal(t, stopCount, 1)
}

// ---------------------------------------------------------------------------
// Multiple plugin services
// ---------------------------------------------------------------------------

// TestReconcileMultiplePluginServices verifies that multiple plugin services
// each produce their own independent OpRunPlugin op.
func TestReconcileMultiplePluginServices(t *testing.T) {
	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"p1": types.ServiceConfig{
				Name:     "p1",
				Provider: &types.ServiceProviderConfig{Type: "aws"},
			},
			"p2": types.ServiceConfig{
				Name:     "p2",
				Provider: &types.ServiceProviderConfig{Type: "gcp"},
			},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. plugin plugin p1  reason: plugin service
2. plugin plugin p2  reason: plugin service
`)
}

// ---------------------------------------------------------------------------
// Orphan edge case
// ---------------------------------------------------------------------------

// TestReconcileOrphanAlreadyStopped verifies that orphan containers in exited
// state still get stop+remove ops (stop is a no-op at execution time).
func TestReconcileOrphanAlreadyStopped(t *testing.T) {
	project := &types.Project{
		Name:     "tp",
		Services: types.Services{},
	}
	orphan := makeContainer("tp", "old", 1, "hash")
	orphan.State = container.StateExited

	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{orphan},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. stop container tp-old-1  reason: orphan container
[1] -> 2. remove container tp-old-1  reason: orphan container
`)
}

// ---------------------------------------------------------------------------
// External volume resolution
// ---------------------------------------------------------------------------

// TestReconcileExternalVolumeResolvedFromContainer verifies that external
// volume names are resolved from running containers' mounts, preventing a
// false "volume configuration changed" recreate.
func TestReconcileExternalVolumeResolvedFromContainer(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "ext", Target: "/data"},
		},
	}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
		Volumes: types.Volumes{
			"ext": types.VolumeConfig{Name: "shared_vol", External: true},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "c1",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ConfigHashLabel:      hash,
						api.ProjectLabel:         "tp",
					},
					Mounts: []container.MountPoint{
						{Type: mmount.TypeVolume, Name: "shared_vol"},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{}, // external vol not in observed
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	// No recreate — external volume name resolved from container mount
	assert.Assert(t, plan.IsEmpty(), "expected empty plan but got:\n%s", plan.String())
}

// ---------------------------------------------------------------------------
// Mixed operations ordering
// ---------------------------------------------------------------------------

// TestReconcileServiceDependsOnMissingNetworkVolumeAndService verifies that
// when a service needs a network, volume, and dependency service all created
// from scratch, the container create depends on all three.
func TestReconcileServiceDependsOnMissingNetworkVolumeAndService(t *testing.T) {
	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db": types.ServiceConfig{Name: "db", Image: "postgres"},
			"web": types.ServiceConfig{
				Name:  "web",
				Image: "nginx",
				Networks: map[string]*types.ServiceNetworkConfig{
					"mynet": nil,
				},
				Volumes: []types.ServiceVolumeConfig{
					{Type: "volume", Source: "data", Target: "/data"},
				},
				DependsOn: types.DependsOnConfig{
					"db": types.ServiceDependency{Condition: "service_started"},
				},
			},
		},
		Networks: types.Networks{
			"mynet": types.NetworkConfig{Name: "tp_mynet"},
		},
		Volumes: types.Volumes{
			"data": types.VolumeConfig{Name: "tp_data"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-db-1  reason: scale up
2. create network tp_mynet  reason: network does not exist
3. create volume tp_data  reason: volume does not exist
[1,2,3] -> 4. create container tp-web-1  reason: scale up
`)
}

// ---------------------------------------------------------------------------
// Inherit flag propagation
// ---------------------------------------------------------------------------

// TestReconcileInheritFlagPropagated verifies that the Inherit option is
// carried through to ContainerOperation on both recreate and scale-up creates.
func TestReconcileInheritFlagPropagated(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx", Scale: intPtr(2)}

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {makeContainer("tp", "web", 1, "stale")},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes:  map[string]ObservedVolume{},
		Orphans:  Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		Inherit:              true,
	})
	assert.NilError(t, err)
	assert.Equal(t, plan.String(), `
1. create container tp-web-1_tp-web-1  reason: config hash changed
2. create container tp-web-2  reason: scale up
[1] -> 3. stop container tp-web-1  reason: config hash changed
[3] -> 4. remove container tp-web-1  reason: config hash changed
[4] -> 5. rename container tp-web-1  reason: config hash changed
[5] -> 6. start container tp-web-1  reason: config hash changed
`)
	// Verify Inherit is set on both create ops
	for _, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ContainerOp != nil {
			assert.Assert(t, op.ContainerOp.Inherit,
				"expected Inherit=true on create op %s", op.Resource)
		}
	}
}

// ---------------------------------------------------------------------------
// Multi-network + simultaneous container recreate
// ---------------------------------------------------------------------------

// TestReconcileMultiNetworkAndContainerRecreate verifies that when a container
// is connected to a recreated network AND also needs recreating due to a config
// hash change, the network-path stop and the recreate-path stop do not collide.
func TestReconcileMultiNetworkAndContainerRecreate(t *testing.T) {
	webSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx:new", // changed image -> config hash diverges
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	oldWebSvc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx:old",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}
	oldHash, err := ServiceHash(oldWebSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"web": webSvc,
		},
		Networks: types.Networks{
			"default": types.NetworkConfig{Name: "tp_default"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"web": {
				{
					ID:    "ctr-web-1",
					Names: []string{"/tp-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "tp",
						api.ConfigHashLabel:      oldHash,
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"tp_default": {NetworkID: "net-1"},
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"default": {ID: "net-1", Name: "tp_default", ConfigHash: "outdated"},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)
	assert.Assert(t, !plan.IsEmpty())

	// Count stop ops for web-1 — must be exactly one despite both network recreate
	// and container recreate wanting to stop it.
	var stopCount int
	for _, op := range plan.Operations {
		if op.Type == OpStopContainer && op.ContainerOp != nil && op.ContainerOp.ContainerName == "tp-web-1" {
			stopCount++
		}
	}
	assert.Equal(t, stopCount, 1, "expected exactly one stop op for tp-web-1")
}

// ---------------------------------------------------------------------------
// Cascading restart with volume-recreated service
// ---------------------------------------------------------------------------

// TestReconcileCascadingRestartWithVolumeRecreatedDep verifies that when a
// service's volume is being recreated (producing a bare create-container op
// without a stop-container), a dependent service with restart: true still gets
// cascading restart ops. The volume-recreated service doesn't go through the
// rename chain, so addCascadingRestarts (which looks for OpRenameContainer)
// should NOT add a cascading restart for it.
func TestReconcileCascadingRestartWithVolumeRecreatedDep(t *testing.T) {
	dbSvc := types.ServiceConfig{
		Name:  "db",
		Image: "postgres",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "dbdata", Target: "/var/lib/data"},
		},
	}
	dbHash, err := ServiceHash(dbSvc)
	assert.NilError(t, err)

	appSvc := types.ServiceConfig{
		Name:  "app",
		Image: "myapp",
		DependsOn: types.DependsOnConfig{
			"db": types.ServiceDependency{
				Condition: "service_started",
				Restart:   true,
			},
		},
	}
	appHash, err := ServiceHash(appSvc)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "tp",
		Services: types.Services{
			"db":  dbSvc,
			"app": appSvc,
		},
		Volumes: types.Volumes{
			"dbdata": types.VolumeConfig{Name: "tp_dbdata"},
		},
	}
	observed := &ObservedState{
		ProjectName: "tp",
		Containers: map[string]Containers{
			"db":  {makeContainer("tp", "db", 1, dbHash)},
			"app": {makeContainer("tp", "app", 1, appHash)},
		},
		Networks: map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"dbdata": {Name: "tp_dbdata", ConfigHash: "outdated-vol-hash"},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// db container gets a create-container op (volume recreated path) — no rename,
	// so addCascadingRestarts won't detect db as "recreated."
	dbCreate, hasCreate := plan.Operations["create-container:tp-db-1"]
	assert.Assert(t, hasCreate, "db should have create-container op from volume recreate")
	assert.Assert(t, dbCreate.ContainerOp != nil)

	// No rename op for db — it goes through the volume-recreated path, not the
	// standard recreate chain.
	_, hasRename := plan.Operations["rename-container:tp-db-1"]
	assert.Assert(t, !hasRename, "volume-recreated container should not have rename op")

	// app should NOT get cascading restart ops because db doesn't have a rename op
	// (cascading restart detection relies on OpRenameContainer).
	_, hasAppStop := plan.Operations["stop-container:tp-app-1"]
	assert.Assert(t, !hasAppStop, "app should not get cascading restart when db is volume-recreated (no rename)")
}

// ---------------------------------------------------------------------------
// Orphan containers with removal ops are "touched"
// ---------------------------------------------------------------------------

// TestReconcileOrphanContainersTouched verifies that orphan containers get
// stop+remove ops, so ContainerTouched returns true for them. This ensures
// emitUntouchedContainerEvents won't emit a "running" event for a container
// that's about to be removed.
func TestReconcileOrphanContainersTouched(t *testing.T) {
	project := &types.Project{
		Name:     "tp",
		Services: types.Services{},
	}
	orphan := makeContainer("tp", "removed-svc", 1, "hash")

	observed := &ObservedState{
		ProjectName: "tp",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{orphan},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers: true,
		Recreate:        api.RecreateDiverged,
		RemoveOrphans:   true,
	})
	assert.NilError(t, err)

	ctrName := getCanonicalContainerName(orphan)

	// The plan should have stop+remove for the orphan
	_, hasStop := plan.Operations["stop-container:"+ctrName]
	assert.Assert(t, hasStop, "orphan should have stop op")
	_, hasRemove := plan.Operations["remove-container:"+ctrName]
	assert.Assert(t, hasRemove, "orphan should have remove op")

	// ContainerTouched should return true for this container
	assert.Assert(t, plan.ContainerTouched(ctrName),
		"orphan with stop+remove ops should be touched")
}

// ---------------------------------------------------------------------------
// TestReconcilePruneCleansDanglingDependsOn verifies that after pruning stale
// network ops for recreated containers, no operation has a DependsOn reference
// to a deleted operation.
// ---------------------------------------------------------------------------

func TestReconcilePruneCleansDanglingDependsOn(t *testing.T) {
	// Setup: container with stale config hash connected to network with stale hash.
	// Both container and network will be recreated.
	svc := types.ServiceConfig{
		Name:  "web",
		Image: "nginx",
		Networks: map[string]*types.ServiceNetworkConfig{
			"default": nil,
		},
	}

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": svc,
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
					ID:    "ctr-web-old12",
					Names: []string{"/testproject-web-1"},
					State: container.StateRunning,
					Labels: map[string]string{
						api.ServiceLabel:         "web",
						api.ContainerNumberLabel: "1",
						api.ProjectLabel:         "testproject",
						api.ConfigHashLabel:      "stale-hash",
					},
					NetworkSettings: &container.NetworkSettingsSummary{
						Networks: map[string]*network.EndpointSettings{
							"testproject_default": {NetworkID: "net-old"},
						},
					},
				},
			},
		},
		Networks: map[string]ObservedNetwork{
			"default": {
				ID:         "net-old",
				Name:       "testproject_default",
				ConfigHash: "outdated-hash",
			},
		},
		Volumes: map[string]ObservedVolume{},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// Verify: every DependsOn in plan points to an existing operation
	for _, op := range plan.Operations {
		for _, depID := range op.DependsOn {
			_, exists := plan.Operations[depID]
			assert.Assert(t, exists, "operation %q has dangling DependsOn reference to %q", op.ID, depID)
		}
	}
}

// ---------------------------------------------------------------------------
// TestReconcileSkipUpToDateVolume verifies that a volume whose config hash
// matches the observed state produces no volume operations.
// ---------------------------------------------------------------------------

func TestReconcileSkipUpToDateVolume(t *testing.T) {
	vol := types.VolumeConfig{Name: "testproject_data"}
	hash, err := VolumeHash(vol)
	assert.NilError(t, err)

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": types.ServiceConfig{Name: "web", Image: "nginx"},
		},
		Volumes: types.Volumes{
			"data": vol,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{},
		Volumes: map[string]ObservedVolume{
			"data": {
				Name:       "testproject_data",
				Driver:     "local",
				ConfigHash: hash,
			},
		},
		Orphans: Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpCreateVolume || op.Type == OpRemoveVolume {
			t.Fatalf("unexpected volume operation: %s", op.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// TestReconcileVolumeRecreateAndConfigHashStale verifies that when a container
// has both a stale config hash AND its volume is being recreated, the volume
// recreate path takes precedence.
// ---------------------------------------------------------------------------

func TestReconcileVolumeRecreateAndConfigHashStale(t *testing.T) {
	svc := types.ServiceConfig{
		Name:  "app",
		Image: "nginx",
		Volumes: []types.ServiceVolumeConfig{
			{Type: "volume", Source: "data", Target: "/data"},
		},
	}

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
						api.ConfigHashLabel:      "stale-container-hash",
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
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// The volume recreate path creates a simple create-container (no rename op),
	// because the volume recreation removes the old container.
	for _, op := range plan.Operations {
		if op.Type == OpRenameContainer && op.ServiceName == "app" {
			t.Fatalf("unexpected rename op for app: volume recreate path should not produce a rename chain")
		}
	}

	// There should be a create-container for app
	found := false
	for _, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ServiceName == "app" {
			found = true
			break
		}
	}
	assert.Assert(t, found, "expected create-container op for app")
}

// ---------------------------------------------------------------------------
// TestReconcileRemovingContainerNoStart verifies that a container in "removing"
// state does not get a start operation.
// ---------------------------------------------------------------------------

func TestReconcileRemovingContainerNoStart(t *testing.T) {
	svc := types.ServiceConfig{Name: "web", Image: "nginx"}
	hash, err := ServiceHash(svc)
	assert.NilError(t, err)

	ctr := makeContainer("testproject", "web", 1, hash)
	ctr.State = "removing"

	project := &types.Project{
		Name: "testproject",
		Services: types.Services{
			"web": svc,
		},
	}
	observed := &ObservedState{
		ProjectName: "testproject",
		Containers:  map[string]Containers{"web": {ctr}},
		Networks:    map[string]ObservedNetwork{},
		Volumes:     map[string]ObservedVolume{},
		Orphans:     Containers{},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpStartContainer && op.ServiceName == "web" {
			t.Fatalf("unexpected start op for container in removing state: %s", op.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// TestReconcileCreateDoesNotIncludeStartOps verifies that when StartContainers
// is false (docker compose create), recreated containers don't get start ops
// and non-running containers don't get start ops.
// ---------------------------------------------------------------------------

func TestReconcileCreateDoesNotIncludeStartOps(t *testing.T) {
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
		StartContainers:      false, // docker compose create
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	for _, op := range plan.Operations {
		if op.Type == OpStartContainer {
			// Network recreation starts are acceptable
			if op.ContainerOp != nil && op.ContainerOp.NetworkRecreate {
				continue
			}
			t.Fatalf("unexpected start op when StartContainers is false: %s (reason: %s)", op.ID, op.Reason)
		}
	}
}

// ---------------------------------------------------------------------------
// TestReconcileOrphansRemovedBeforeServiceCreation verifies that orphan
// container removal operations are dependencies of service container creation.
// ---------------------------------------------------------------------------

func TestReconcileOrphansRemovedBeforeServiceCreation(t *testing.T) {
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
			makeContainer("testproject", "old", 1, "hash-old"),
		},
	}

	plan, err := Reconcile(project, observed, ReconcileOptions{
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
		RemoveOrphans:        true,
	})
	assert.NilError(t, err)

	// Find the create-container op for web
	var createOp *Operation
	for _, op := range plan.Operations {
		if op.Type == OpCreateContainer && op.ServiceName == "web" {
			createOp = op
			break
		}
	}
	assert.Assert(t, createOp != nil, "expected create-container op for web")

	// Find the remove-container op for orphan
	orphanRemoveID := "remove-container:testproject-old-1"
	_, hasRemove := plan.Operations[orphanRemoveID]
	assert.Assert(t, hasRemove, "expected remove-container op for orphan")

	// Assert create-container depends on orphan removal
	found := false
	for _, dep := range createOp.DependsOn {
		if dep == orphanRemoveID {
			found = true
			break
		}
	}
	assert.Assert(t, found, "create-container:web should depend on orphan removal, got deps: %v", createOp.DependsOn)
}

// ---------------------------------------------------------------------------
// TestReconcileCascadingRestartStopDependsOnRename verifies that when a
// dependent service is restarted due to dependency recreation, its stop
// operation depends on the dependency's rename (completion of recreation).
// ---------------------------------------------------------------------------

func TestReconcileCascadingRestartStopDependsOnRename(t *testing.T) {
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
		StartContainers:      true,
		Recreate:             api.RecreateDiverged,
		RecreateDependencies: api.RecreateDiverged,
	})
	assert.NilError(t, err)

	// Find stop-container:testproject-web-1
	stopOp, exists := plan.Operations["stop-container:testproject-web-1"]
	assert.Assert(t, exists, "expected stop-container op for web")

	// It should depend on the rename op for db
	renameID := "rename-container:testproject-db-1"
	_, hasRename := plan.Operations[renameID]
	assert.Assert(t, hasRename, "expected rename-container op for db")

	found := false
	for _, dep := range stopOp.DependsOn {
		if dep == renameID {
			found = true
			break
		}
	}
	assert.Assert(t, found, "stop-container:web should depend on rename-container:db, got deps: %v", stopOp.DependsOn)
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
