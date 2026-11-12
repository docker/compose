/*
   Copyright 2026 Docker Compose CLI authors

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
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/client"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/docker/compose/v5/internal/planning"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

// TestEmittersReportOnTheRowTheyOpened pins that the Working, Done and Error
// events of a node all name the same resource: the one a consumer keying on
// Resource.ID saw opened. The plan's internal ResourceID (service:web:1,
// network:front) is no resource a consumer ever heard of.
func TestEmittersReportOnTheRowTheyOpened(t *testing.T) {
	web := startTestSummary("web")
	tests := []struct {
		name string
		op   planning.Operation
		row  string
	}{
		{"create container", planning.Operation{Type: planning.OpCreateContainer, ResourceID: "service:web:1", Name: "test-web-1"}, "Container test-web-1"},
		{"start container", planning.Operation{Type: planning.OpStartContainer, ResourceID: "service:web:1", Container: &web}, "Container test-web-1"},
		{"stop container", planning.Operation{Type: planning.OpStopContainer, ResourceID: "service:web:1", Container: &web}, "Container test-web-1"},
		{"remove container", planning.Operation{Type: planning.OpRemoveContainer, ResourceID: "service:web:1", Container: &web}, "Container test-web-1"},
		{"create network", planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:front", Name: "test_front"}, "Network test_front"},
		{"remove network", planning.Operation{Type: planning.OpRemoveNetwork, ResourceID: "network:front", Name: "test_front"}, "Network test_front"},
		{"create volume", planning.Operation{Type: planning.OpCreateVolume, ResourceID: "volume:data", Name: "test_data"}, "Volume test_data"},
		{"remove volume", planning.Operation{Type: planning.OpRemoveVolume, ResourceID: "volume:data", Name: "test_data"}, "Volume test_data"},
	}
	endings := []struct {
		name   string
		emit   func(*planning.PlanNode, api.EventProcessor)
		status api.EventStatus
	}{
		{"done", emitDoneEvent, api.Done},
		{"error", func(n *planning.PlanNode, e api.EventProcessor) { emitErrorEvent(n, e, errors.New("boom")) }, api.Error},
	}
	for _, tt := range tests {
		for _, end := range endings {
			t.Run(tt.name+"/"+end.name, func(t *testing.T) {
				node := &planning.PlanNode{Operation: tt.op}
				events := &capturingEvents{}
				emitStartEvent(node, events)
				end.emit(node, events)

				assert.Equal(t, len(events.resources), 2, "%v", events.resources)
				for _, e := range events.resources {
					assert.Equal(t, e.ID, tt.row)
				}
				assert.Equal(t, events.resources[0].Status, api.Working)
				assert.Equal(t, events.resources[1].Status, end.status)
			})
		}
	}
}

// TestEmitErrorEventOfANodeWithoutARowOfItsOwn pins that a node with no row of
// its own opens and closes none, and where its error goes: on the row of the
// container it acts on, else under the plan's ResourceID, as there is no other
// name for it.
func TestEmitErrorEventOfANodeWithoutARowOfItsOwn(t *testing.T) {
	web := startTestSummary("web")
	tests := []struct {
		name string
		op   planning.Operation
		row  string
	}{
		{"connect network", planning.Operation{Type: planning.OpConnectNetwork, ResourceID: "service:web:1", Container: &web}, "Container test-web-1"},
		{"disconnect network", planning.Operation{Type: planning.OpDisconnectNetwork, ResourceID: "service:web:1", Container: &web}, "Container test-web-1"},
		{"hook", planning.Operation{Type: planning.OpRunPreStart, ResourceID: "hook:web:pre_start:0"}, "hook:web:pre_start:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &planning.PlanNode{Operation: tt.op}
			events := &capturingEvents{}
			emitStartEvent(node, events)
			emitDoneEvent(node, events)
			assert.Equal(t, len(events.resources), 0, "%v", events.resources)

			emitErrorEvent(node, events, errors.New("boom"))
			assert.DeepEqual(t, events.resources, []api.Resource{{ID: tt.row, Status: api.Error, Text: "boom"}})
		})
	}
}

// Expectations on the daemon calls a node makes, each one returning err.
func networkCreate(name string, err error) func(*mocks.MockAPIClient) {
	return func(m *mocks.MockAPIClient) {
		m.EXPECT().NetworkCreate(gomock.Any(), name, gomock.Any()).Return(client.NetworkCreateResult{}, err)
	}
}

func volumeCreate(err error) func(*mocks.MockAPIClient) {
	return func(m *mocks.MockAPIClient) {
		m.EXPECT().VolumeCreate(gomock.Any(), gomock.Any()).Return(client.VolumeCreateResult{}, err)
	}
}

// TestExecutePlanNetworkAndVolumeRowsOpenAndCloseOnce pins that a network or a
// volume operation reports its Working event and its final one once each, on
// the row of the resource. The operation itself used to report them too, on top
// of the executor.
func TestExecutePlanNetworkAndVolumeRowsOpenAndCloseOnce(t *testing.T) {
	createFront := planning.Operation{Type: planning.OpCreateNetwork, ResourceID: "network:front", Name: "test_front", Network: &types.NetworkConfig{Name: "test_front"}}
	createData := planning.Operation{Type: planning.OpCreateVolume, ResourceID: "volume:data", Name: "test_data", Volume: &types.VolumeConfig{Name: "test_data"}}
	tests := []struct {
		name   string
		op     planning.Operation
		expect func(*mocks.MockAPIClient)
		row    string
		want   []string // the row's texts; the last one is the error when the node fails
		failed bool
	}{
		{
			name:   "network created",
			op:     createFront,
			expect: networkCreate("test_front", nil),
			row:    "Network test_front",
			want:   []string{"Creating", "Created"},
		},
		{
			name:   "network created by a concurrent command",
			op:     createFront,
			expect: networkCreate("test_front", conflictError{}),
			row:    "Network test_front",
			want:   []string{"Creating", "Created"},
		},
		{
			name:   "network failed",
			op:     createFront,
			expect: networkCreate("test_front", errors.New("plugin not found")),
			row:    "Network test_front",
			want:   []string{"Creating", "failed to create network test_front: plugin not found"},
			failed: true,
		},
		{
			name:   "volume created",
			op:     createData,
			expect: volumeCreate(nil),
			row:    "Volume test_data",
			want:   []string{"Creating", "Created"},
		},
		{
			name:   "volume failed",
			op:     createData,
			expect: volumeCreate(errors.New("driver not found")),
			row:    "Volume test_data",
			want:   []string{"Creating", "driver not found"},
			failed: true,
		},
		{
			name: "network removal failed",
			op:   planning.Operation{Type: planning.OpRemoveNetwork, ResourceID: "network:front", Name: "test_front"},
			expect: func(m *mocks.MockAPIClient) {
				m.EXPECT().NetworkRemove(gomock.Any(), "test_front", gomock.Any()).
					Return(client.NetworkRemoveResult{}, errors.New("has active endpoints"))
			},
			row:    "Network test_front",
			want:   []string{"Removing", "has active endpoints"},
			failed: true,
		},
		{
			name: "volume removal failed",
			op:   planning.Operation{Type: planning.OpRemoveVolume, ResourceID: "volume:data", Name: "test_data"},
			expect: func(m *mocks.MockAPIClient) {
				m.EXPECT().VolumeRemove(gomock.Any(), "test_data", gomock.Any()).
					Return(client.VolumeRemoveResult{}, errors.New("volume in use"))
			},
			row:    "Volume test_data",
			want:   []string{"Removing", "volume in use"},
			failed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, apiClient, recorder := newStartPhaseTestService(t)
			tt.expect(apiClient)

			plan := &planning.Plan{}
			plan.AddNode(tt.op, "")
			exec := svc.newPlanExecutor(&types.Project{Name: "test"}, emptyObservedState("test"))
			err := exec.run(t.Context(), plan)

			if tt.failed {
				assert.ErrorContains(t, err, tt.want[len(tt.want)-1])
			} else {
				assert.NilError(t, err)
			}
			assert.DeepEqual(t, recorder.byID, map[string][]string{tt.row: tt.want})
		})
	}
}

// TestExecutePlanFailedContainerCreationEndsItsRow pins that a container the
// daemon rejects ends the row that was opened for it, instead of leaving it on
// Creating and reporting under the plan's service:web:1.
func TestExecutePlanFailedContainerCreationEndsItsRow(t *testing.T) {
	svc, apiClient, recorder := newStartPhaseTestService(t)

	service := types.ServiceConfig{Name: "web"}
	project := &types.Project{Name: "test", Services: types.Services{"web": service}}
	apiClient.EXPECT().ContainerCreate(gomock.Any(), gomock.Any()).
		Return(client.ContainerCreateResult{}, errors.New("Minimum memory limit allowed is 6MB"))

	plan := &planning.Plan{}
	plan.AddNode(planning.Operation{
		Type:       planning.OpCreateContainer,
		ResourceID: "service:web:1",
		Service:    &service,
		Name:       "test-web-1",
		Number:     1,
	}, "")
	exec := svc.newPlanExecutor(project, emptyObservedState("test"))
	assert.ErrorContains(t, exec.run(t.Context(), plan), "Minimum memory limit")

	assert.Equal(t, len(recorder.byID), 1, "%v", recorder.byID)
	row := recorder.byID["Container test-web-1"]
	assert.Equal(t, len(row), 2, "%v", row)
	assert.Equal(t, row[0], "Creating")
	assert.Assert(t, cmp.Contains(row[1], "Minimum memory limit allowed is 6MB"))
}
