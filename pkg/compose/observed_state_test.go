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

	"gotest.tools/v3/assert"
)

func TestObservedStateTypes(t *testing.T) {
	net := ObservedNetwork{
		ID:         "net123",
		Name:       "myproject_default",
		Driver:     "bridge",
		Labels:     map[string]string{"com.docker.compose.network": "default"},
		ConfigHash: "abc123",
	}
	assert.Equal(t, net.ID, "net123")
	assert.Equal(t, net.Name, "myproject_default")
	assert.Equal(t, net.Driver, "bridge")
	assert.Equal(t, net.ConfigHash, "abc123")
	assert.Equal(t, net.Labels["com.docker.compose.network"], "default")

	vol := ObservedVolume{
		Name:       "myproject_data",
		Driver:     "local",
		Labels:     map[string]string{"com.docker.compose.volume": "data"},
		ConfigHash: "def456",
	}
	assert.Equal(t, vol.Name, "myproject_data")
	assert.Equal(t, vol.Driver, "local")
	assert.Equal(t, vol.ConfigHash, "def456")
	assert.Equal(t, vol.Labels["com.docker.compose.volume"], "data")

	state := ObservedState{
		ProjectName: "myproject",
		Containers:  map[string]Containers{},
		Networks:    map[string]ObservedNetwork{"default": net},
		Volumes:     map[string]ObservedVolume{"data": vol},
		Orphans:     Containers{},
	}
	assert.Equal(t, state.ProjectName, "myproject")
	assert.Equal(t, len(state.Networks), 1)
	assert.Equal(t, len(state.Volumes), 1)
	assert.Equal(t, state.Networks["default"].ID, "net123")
	assert.Equal(t, state.Volumes["data"].Name, "myproject_data")
}

func TestReconciliationPlanRoots(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"create-network:mynet": {
				ID:   "create-network:mynet",
				Type: OpCreateNetwork,
			},
			"create-volume:myvol": {
				ID:   "create-volume:myvol",
				Type: OpCreateVolume,
			},
			"create-container:web-1": {
				ID:        "create-container:web-1",
				Type:      OpCreateContainer,
				DependsOn: []string{"create-network:mynet", "create-volume:myvol"},
			},
			"start-container:db-1": {
				ID:        "start-container:db-1",
				Type:      OpStartContainer,
				DependsOn: []string{},
			},
		},
		Dependents: map[string][]string{},
	}

	roots := plan.Roots()
	// Roots should be the operations with empty DependsOn: network, volume, and start-container
	assert.Equal(t, len(roots), 3)
	// Roots are sorted by ID
	assert.Equal(t, roots[0].ID, "create-network:mynet")
	assert.Equal(t, roots[1].ID, "create-volume:myvol")
	assert.Equal(t, roots[2].ID, "start-container:db-1")
}

func TestReconciliationPlanIsEmpty(t *testing.T) {
	emptyPlan := &ReconciliationPlan{
		Operations: map[string]*Operation{},
		Dependents: map[string][]string{},
	}
	assert.Assert(t, emptyPlan.IsEmpty())

	nonEmptyPlan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"create-network:mynet": {
				ID:   "create-network:mynet",
				Type: OpCreateNetwork,
			},
		},
		Dependents: map[string][]string{},
	}
	assert.Assert(t, !nonEmptyPlan.IsEmpty())
}

func TestReconciliationPlanString(t *testing.T) {
	emptyPlan := &ReconciliationPlan{
		Operations: map[string]*Operation{},
		Dependents: map[string][]string{},
	}
	assert.Equal(t, emptyPlan.String(), "(empty plan)")

	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"create-network:mynet": {
				ID:       "create-network:mynet",
				Type:     OpCreateNetwork,
				Resource: "mynet",
				NetworkOp: &NetworkOperation{
					NetworkKey: "default",
				},
				Reason: "network does not exist",
			},
			"create-container:web-1": {
				ID:          "create-container:web-1",
				Type:        OpCreateContainer,
				ServiceName: "web",
				Resource:    "web-1",
				ContainerOp: &ContainerOperation{
					ContainerName:   "web-1",
					ContainerNumber: 1,
				},
				DependsOn: []string{"create-network:mynet"},
				Reason:    "scale up",
			},
		},
		Dependents: map[string][]string{
			"create-network:mynet": {"create-container:web-1"},
		},
	}
	expected := `
1. create network mynet  reason: network does not exist
[1] -> 2. create container web-1  reason: scale up
`
	assert.Equal(t, plan.String(), expected)
}

func TestContainerTouchedMatchesContainerOps(t *testing.T) {
	plan := &ReconciliationPlan{
		Operations: map[string]*Operation{
			"stop-container:web-1": {
				ID:   "stop-container:web-1",
				Type: OpStopContainer,
				ContainerOp: &ContainerOperation{
					ContainerName: "web-1",
				},
			},
			"create-container:tmpname_web-2": {
				ID:   "create-container:tmpname_web-2",
				Type: OpCreateContainer,
				ContainerOp: &ContainerOperation{
					ContainerName: "tmpname_web-2",
				},
			},
			"rename-container:web-2": {
				ID:   "rename-container:web-2",
				Type: OpRenameContainer,
				RenameOp: &RenameOperation{
					CurrentName: "tmpname_web-2",
					NewName:     "web-2",
				},
				// No ContainerOp — rename ops use RenameOp
			},
			"remove-container:db-1": {
				ID:   "remove-container:db-1",
				Type: OpRemoveContainer,
				ContainerOp: &ContainerOperation{
					ContainerName: "db-1",
				},
			},
			"create-network:mynet": {
				ID:   "create-network:mynet",
				Type: OpCreateNetwork,
				NetworkOp: &NetworkOperation{
					NetworkKey: "default",
				},
			},
		},
		Dependents: map[string][]string{},
	}

	// Containers with ContainerOp are touched
	assert.Assert(t, plan.ContainerTouched("web-1"), "stop op should mark web-1 as touched")
	assert.Assert(t, plan.ContainerTouched("tmpname_web-2"), "create op should mark tmpname_web-2 as touched")
	assert.Assert(t, plan.ContainerTouched("db-1"), "remove op should mark db-1 as touched")

	// Rename ops have RenameOp, not ContainerOp — rename target is NOT touched via ContainerOp
	assert.Assert(t, !plan.ContainerTouched("web-2"), "rename op (RenameOp only) should not mark web-2 as touched")

	// Network ops are not container ops
	assert.Assert(t, !plan.ContainerTouched("mynet"), "network op should not match container name")

	// Non-existent container
	assert.Assert(t, !plan.ContainerTouched("nonexistent"), "unknown container should not be touched")
}
