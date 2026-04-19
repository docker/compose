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

func TestOperationTypeString(t *testing.T) {
	tests := []struct {
		op   OperationType
		want string
	}{
		{OpCreateNetwork, "CreateNetwork"},
		{OpRemoveNetwork, "RemoveNetwork"},
		{OpDisconnectNetwork, "DisconnectNetwork"},
		{OpConnectNetwork, "ConnectNetwork"},
		{OpCreateVolume, "CreateVolume"},
		{OpRemoveVolume, "RemoveVolume"},
		{OpCreateContainer, "CreateContainer"},
		{OpStartContainer, "StartContainer"},
		{OpStopContainer, "StopContainer"},
		{OpRemoveContainer, "RemoveContainer"},
		{OpRenameContainer, "RenameContainer"},
		{OperationType(999), "Unknown(999)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.op.String(), tt.want)
	}
}

func TestPlanStringEmpty(t *testing.T) {
	p := &Plan{}
	assert.Equal(t, p.String(), "")
	assert.Assert(t, p.IsEmpty())
}

func TestPlanStringNoDeps(t *testing.T) {
	p := &Plan{}
	p.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Cause:      "not found",
	}, "")
	p.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: "volume:data",
		Cause:      "not found",
	}, "")

	expected := "[] -> #1 network:default, CreateNetwork, not found\n" +
		"[] -> #2 volume:data, CreateVolume, not found\n"
	assert.Equal(t, p.String(), expected)
	assert.Assert(t, !p.IsEmpty())
}

func TestPlanStringWithDeps(t *testing.T) {
	p := &Plan{}
	nw := p.addNode(Operation{
		Type:       OpCreateNetwork,
		ResourceID: "network:default",
		Cause:      "not found",
	}, "")
	vol := p.addNode(Operation{
		Type:       OpCreateVolume,
		ResourceID: "volume:data",
		Cause:      "not found",
	}, "")
	p.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:db:1",
		Cause:      "no existing container",
	}, "", nw, vol)

	expected := "[] -> #1 network:default, CreateNetwork, not found\n" +
		"[] -> #2 volume:data, CreateVolume, not found\n" +
		"[1,2] -> #3 service:db:1, CreateContainer, no existing container\n"
	assert.Equal(t, p.String(), expected)
}

func TestPlanStringWithGroup(t *testing.T) {
	p := &Plan{}
	create := p.addNode(Operation{
		Type:       OpCreateContainer,
		ResourceID: "service:web:1",
		Cause:      "config hash changed (tmpName)",
	}, "recreate:web:1")
	stop := p.addNode(Operation{
		Type:       OpStopContainer,
		ResourceID: "service:web:1",
		Cause:      "replaced by #1",
	}, "recreate:web:1", create)
	remove := p.addNode(Operation{
		Type:       OpRemoveContainer,
		ResourceID: "service:web:1",
		Cause:      "replaced by #1",
	}, "recreate:web:1", stop)
	p.addNode(Operation{
		Type:       OpRenameContainer,
		ResourceID: "service:web:1",
		Cause:      "finalize recreate",
	}, "recreate:web:1", remove)

	expected := "[] -> #1 service:web:1, CreateContainer, config hash changed (tmpName) [recreate:web:1]\n" +
		"[1] -> #2 service:web:1, StopContainer, replaced by #1 [recreate:web:1]\n" +
		"[2] -> #3 service:web:1, RemoveContainer, replaced by #1 [recreate:web:1]\n" +
		"[3] -> #4 service:web:1, RenameContainer, finalize recreate [recreate:web:1]\n"
	assert.Equal(t, p.String(), expected)
}

func TestPlanAddNodeAutoIncrements(t *testing.T) {
	p := &Plan{}
	n1 := p.addNode(Operation{Type: OpCreateNetwork, ResourceID: "a", Cause: "x"}, "")
	n2 := p.addNode(Operation{Type: OpCreateVolume, ResourceID: "b", Cause: "y"}, "")
	n3 := p.addNode(Operation{Type: OpCreateContainer, ResourceID: "c", Cause: "z"}, "", n1, n2)

	assert.Equal(t, n1.ID, 1)
	assert.Equal(t, n2.ID, 2)
	assert.Equal(t, n3.ID, 3)
	assert.Equal(t, len(n3.DependsOn), 2)
	assert.Equal(t, n3.DependsOn[0].ID, 1)
	assert.Equal(t, n3.DependsOn[1].ID, 2)
}
