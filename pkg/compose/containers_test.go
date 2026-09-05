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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// TestIsOrphaned pins the orphan definition: a container is orphaned when it
// is a one-off that FINISHED its task (a still-running `compose run` is a live
// session `up --remove-orphans` must never kill), or when it carries the
// project labels but its service is not defined by the compose model (enabled
// or disabled) — the typical leftover after the compose file was edited.
func TestIsOrphaned(t *testing.T) {
	project := &types.Project{
		Name: "p",
		Services: types.Services{
			"web": {Name: "web"},
		},
		DisabledServices: types.Services{
			"debug": {Name: "debug"},
		},
	}
	ctr := func(service, oneOff string, state container.ContainerState) container.Summary {
		labels := map[string]string{api.ServiceLabel: service, api.ProjectLabel: "p"}
		if oneOff != "" {
			labels[api.OneoffLabel] = oneOff
		}
		return container.Summary{Labels: labels, State: state}
	}
	pred := isOrphaned(project)

	for _, tc := range []struct {
		name string
		c    container.Summary
		want bool
	}{
		{"service replica running", ctr("web", "False", container.StateRunning), false},
		{"service replica exited", ctr("web", "False", container.StateExited), false},
		{"disabled-profile service", ctr("debug", "False", container.StateExited), false},
		{"service removed from the model", ctr("old", "False", container.StateRunning), true},
		{"one-off of a declared service, exited", ctr("web", "True", container.StateExited), true},
		{"one-off of a declared service, RUNNING: a live session, not an orphan", ctr("web", "True", container.StateRunning), false},
		{"one-off of a removed service, dead", ctr("old", "True", container.StateDead), true},
		{"one-off of a removed service, RUNNING: still a live session", ctr("old", "True", container.StateRunning), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, pred(tc.c), tc.want)
		})
	}
}
