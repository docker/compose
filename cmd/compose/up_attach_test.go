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
	"sort"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestUpAttachedServices(t *testing.T) {
	noAttach := false
	web := types.ServiceConfig{Name: "web"}
	web.DependsOn = types.DependsOnConfig{"db": {Condition: types.ServiceConditionStarted, Required: true}}
	project := &types.Project{
		Name: "test",
		Services: types.Services{
			"web":    web,
			"db":     {Name: "db"},
			"worker": {Name: "worker", Attach: &noAttach},
		},
	}

	tests := []struct {
		name     string
		opts     upOptions
		services []string
		expected []string
		err      string
	}{
		{name: "every service by default", expected: []string{"db", "web"}, services: []string{"web", "db", "worker"}},
		{name: "a service that opts out in the Compose file is not attached", services: []string{"web", "worker"}, expected: []string{"web"}},
		{name: "dependencies are left out by default", services: []string{"web"}, expected: []string{"web"}},
		{name: "dependencies are attached with --attach-dependencies", opts: upOptions{attachDependencies: true}, services: []string{"web"}, expected: []string{"db", "web"}},
		{name: "--no-attach excludes a service", opts: upOptions{noAttach: []string{"db"}}, services: []string{"web", "db"}, expected: []string{"web"}},
		{name: "--attach is used as is, whatever the Compose file says", opts: upOptions{attach: []string{"worker"}}, services: []string{"web", "worker"}, expected: []string{"worker"}},
		{name: "--no-attach also filters --attach", opts: upOptions{attach: []string{"web", "db"}, noAttach: []string{"db"}}, services: []string{"web", "db"}, expected: []string{"web"}},
		{name: "--attach to a service that is not part of up", opts: upOptions{attach: []string{"missing"}}, services: []string{"web"}, err: "cannot attach to services not included in up: missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attached, err := tt.opts.attachedServices(project, tt.services)
			if tt.err != "" {
				assert.Error(t, err, tt.err)
				return
			}
			assert.NilError(t, err)
			sort.Strings(attached)
			assert.DeepEqual(t, attached, tt.expected)
		})
	}
}
