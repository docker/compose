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
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestRunApply_WarnsWhenServicePortsDropped(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14229
	// run drops the model's ports unless --service-ports is set, and used to
	// do that silently (debug only).
	ports := []types.ServicePortConfig{{Published: "8000", Target: 8000, Protocol: "tcp"}}
	tests := []struct {
		name         string
		ports        []types.ServicePortConfig
		servicePorts bool
		publish      []string
		wantWarn     bool
		wantPortLen  int
	}{
		{
			name:        "drops compose ports and warns",
			ports:       ports,
			wantWarn:    true,
			wantPortLen: 0,
		},
		{
			name:         "keeps compose ports with --service-ports",
			ports:        ports,
			servicePorts: true,
			wantWarn:     false,
			wantPortLen:  1,
		},
		{
			name:        "no ports defined",
			wantWarn:    false,
			wantPortLen: 0,
		},
		{
			name:        "replaces compose ports with --publish",
			ports:       ports,
			publish:     []string{"8081:80"},
			wantWarn:    false,
			wantPortLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := &types.Project{
				Services: types.Services{
					"web": {Name: "web", Ports: tt.ports},
				},
			}
			opts := runOptions{
				Service:      "web",
				servicePorts: tt.servicePorts,
				publish:      tt.publish,
			}

			var got *types.Project
			messages := captureWarnings(t, func() {
				var err error
				got, err = opts.apply(project)
				assert.NilError(t, err)
			})

			if tt.wantWarn {
				assert.Equal(t, len(messages), 1)
				assert.Assert(t, strings.Contains(messages[0], `service "web"`))
				assert.Assert(t, strings.Contains(messages[0], "--service-ports"))
			} else {
				assert.Equal(t, len(messages), 0)
			}
			assert.Equal(t, len(got.Services["web"].Ports), tt.wantPortLen)
		})
	}
}
