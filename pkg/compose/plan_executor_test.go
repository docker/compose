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
	"bytes"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	containerType "github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"
)

// ---------------------------------------------------------------------------
// topologicalSort tests
// ---------------------------------------------------------------------------

func TestTopologicalSortLinearChain(t *testing.T) {
	// A→B→C: A depends on B, B depends on C (C runs first)
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"A": {ID: "A", DependsOn: []string{"B"}},
			"B": {ID: "B", DependsOn: []string{"C"}},
			"C": {ID: "C", DependsOn: nil},
		},
		Dependents: map[string][]string{},
	}
	buildReverseEdges(plan)

	sorted, err := topologicalSort(plan)
	assert.NilError(t, err)
	assert.Equal(t, len(sorted), 3)
	assert.Equal(t, sorted[0].ID, "C")
	assert.Equal(t, sorted[1].ID, "B")
	assert.Equal(t, sorted[2].ID, "A")
}

func TestTopologicalSortDiamond(t *testing.T) {
	// D has no deps, B and C depend on D, A depends on B and C
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"A": {ID: "A", DependsOn: []string{"B", "C"}},
			"B": {ID: "B", DependsOn: []string{"D"}},
			"C": {ID: "C", DependsOn: []string{"D"}},
			"D": {ID: "D", DependsOn: nil},
		},
		Dependents: map[string][]string{},
	}
	buildReverseEdges(plan)

	sorted, err := topologicalSort(plan)
	assert.NilError(t, err)
	assert.Equal(t, len(sorted), 4)

	// D must come before B and C, A must be last
	pos := map[string]int{}
	for i, op := range sorted {
		pos[op.ID] = i
	}
	assert.Assert(t, pos["D"] < pos["B"], "D should come before B")
	assert.Assert(t, pos["D"] < pos["C"], "D should come before C")
	assert.Assert(t, pos["B"] < pos["A"], "B should come before A")
	assert.Assert(t, pos["C"] < pos["A"], "C should come before A")
}

func TestTopologicalSortDetectsCycle(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"A": {ID: "A", DependsOn: []string{"B"}},
			"B": {ID: "B", DependsOn: []string{"A"}},
		},
		Dependents: map[string][]string{},
	}
	buildReverseEdges(plan)

	_, err := topologicalSort(plan)
	assert.Assert(t, err != nil, "expected cycle error")
	assert.Assert(t, contains(err.Error(), "cycle"), "error should mention cycle, got: %s", err.Error())
}

func TestTopologicalSortSingleNode(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"only": {ID: "only", DependsOn: nil},
		},
		Dependents: map[string][]string{},
	}

	sorted, err := topologicalSort(plan)
	assert.NilError(t, err)
	assert.Equal(t, len(sorted), 1)
	assert.Equal(t, sorted[0].ID, "only")
}

func TestTopologicalSortIndependentRoots(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"C": {ID: "C", DependsOn: nil},
			"A": {ID: "A", DependsOn: nil},
			"B": {ID: "B", DependsOn: nil},
		},
		Dependents: map[string][]string{},
	}

	sorted, err := topologicalSort(plan)
	assert.NilError(t, err)
	assert.Equal(t, len(sorted), 3)
	// Independent roots are sorted alphabetically
	assert.Equal(t, sorted[0].ID, "A")
	assert.Equal(t, sorted[1].ID, "B")
	assert.Equal(t, sorted[2].ID, "C")
}

// ---------------------------------------------------------------------------
// resolveServiceReferences tests
// ---------------------------------------------------------------------------

func TestResolveVolumeFromService(t *testing.T) {
	es := newExecutionState()
	es.addContainer("db", containerType.Summary{ID: "db-container-123"})

	service := &types.ServiceConfig{
		Name:        "web",
		VolumesFrom: []string{"db"},
	}

	err := es.resolveServiceReferences(service)
	assert.NilError(t, err)
	assert.Equal(t, service.VolumesFrom[0], "db-container-123")
}

func TestResolveVolumeFromContainerPrefix(t *testing.T) {
	es := newExecutionState()

	service := &types.ServiceConfig{
		Name:        "web",
		VolumesFrom: []string{"container:abc123"},
	}

	err := es.resolveServiceReferences(service)
	assert.NilError(t, err)
	assert.Equal(t, service.VolumesFrom[0], "abc123")
}

func TestResolveVolumeFromMissingService(t *testing.T) {
	es := newExecutionState()

	service := &types.ServiceConfig{
		Name:        "web",
		VolumesFrom: []string{"missing"},
	}

	err := es.resolveServiceReferences(service)
	assert.Assert(t, err != nil, "expected error for missing service")
	assert.Assert(t, contains(err.Error(), "missing"), "error should mention missing service")
}

func TestResolveSharedNetworkMode(t *testing.T) {
	es := newExecutionState()
	es.addContainer("db", containerType.Summary{ID: "db-container-456"})

	service := &types.ServiceConfig{
		Name:        "web",
		NetworkMode: "service:db",
	}

	err := es.resolveServiceReferences(service)
	assert.NilError(t, err)
	assert.Equal(t, service.NetworkMode, "container:db-container-456")
}

func TestResolveSharedIpc(t *testing.T) {
	es := newExecutionState()
	es.addContainer("db", containerType.Summary{ID: "db-container-789"})

	service := &types.ServiceConfig{
		Name: "web",
		Ipc:  "service:db",
	}

	err := es.resolveServiceReferences(service)
	assert.NilError(t, err)
	assert.Equal(t, service.Ipc, "container:db-container-789")
}

func TestResolveSharedPid(t *testing.T) {
	es := newExecutionState()
	es.addContainer("db", containerType.Summary{ID: "db-container-000"})

	service := &types.ServiceConfig{
		Name: "web",
		Pid:  "service:db",
	}

	err := es.resolveServiceReferences(service)
	assert.NilError(t, err)
	assert.Equal(t, service.Pid, "container:db-container-000")
}

// ---------------------------------------------------------------------------
// DisplayPlan tests
// ---------------------------------------------------------------------------

func TestDisplayPlanEmpty(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{},
		Dependents: map[string][]string{},
	}

	var buf bytes.Buffer
	err := DisplayPlan(plan, &buf)
	assert.NilError(t, err)
	assert.Equal(t, buf.String(), "")
}

func TestDisplayPlanGroupsByCategory(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"create-network:mynet": {
				ID:       "create-network:mynet",
				Type:     OpCreateNetwork,
				Resource: "mynet",
				NetworkOp: &NetworkOperation{
					NetworkKey: "mynet",
				},
				Reason: "network does not exist",
			},
			"create-volume:myvol": {
				ID:       "create-volume:myvol",
				Type:     OpCreateVolume,
				Resource: "myvol",
				VolumeOp: &VolumeOperation{
					VolumeKey: "myvol",
				},
				Reason: "volume does not exist",
			},
			"create-container:web-1": {
				ID:          "create-container:web-1",
				Type:        OpCreateContainer,
				ServiceName: "web",
				Resource:    "web-1",
				ContainerOp: &ContainerOperation{
					ContainerName: "web-1",
				},
				DependsOn: []string{"create-network:mynet", "create-volume:myvol"},
				Reason:    "scale up",
			},
		},
		Dependents: map[string][]string{},
	}
	buildReverseEdges(plan)

	var buf bytes.Buffer
	err := DisplayPlan(plan, &buf)
	assert.NilError(t, err)

	output := buf.String()
	assert.Assert(t, contains(output, "Networks:"), "output should contain Networks section")
	assert.Assert(t, contains(output, "Volumes:"), "output should contain Volumes section")
	assert.Assert(t, contains(output, "Services:"), "output should contain Services section")
}

// contains is a small helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
