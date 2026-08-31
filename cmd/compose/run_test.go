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
	"gotest.tools/v3/assert"
)

func TestMaterializeManualJob(t *testing.T) {
	yes, no := true, false
	base := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"db": {Name: "db", ContainerSpec: types.ContainerSpec{Image: "postgres"}},
			},
			Jobs: types.Jobs{
				"migrate": {
					Name:          "migrate",
					Triggers:      &types.TriggerConfig{Manual: &yes},
					ContainerSpec: types.ContainerSpec{Image: "migrator", Command: types.ShellCommand{"migrate"}},
					WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"db": {Condition: types.ServiceConditionStarted, Required: true}}},
				},
				"backup": {
					Name:          "backup",
					Triggers:      &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}},
					ContainerSpec: types.ContainerSpec{Image: "backup-tool"},
				},
				"rotation": {
					Name:     "rotation",
					Triggers: &types.TriggerConfig{Manual: &no, Schedule: []types.ScheduleConfig{{Cron: "0 3 1 * *"}}},
				},
			},
		}
	}

	t.Run("a service name passes through", func(t *testing.T) {
		p := base()
		got, err := materializeManualJob(p, "db")
		assert.NilError(t, err)
		assert.Equal(t, got, p)
	})

	t.Run("a manual job materializes as a service with its spec and deps", func(t *testing.T) {
		got, err := materializeManualJob(base(), "migrate")
		assert.NilError(t, err)
		svc, err := got.GetService("migrate")
		assert.NilError(t, err)
		assert.Equal(t, svc.Image, "migrator")
		assert.DeepEqual(t, []string(svc.Command), []string{"migrate"})
		_, hasDep := svc.DependsOn["db"]
		assert.Check(t, hasDep)
		// the dependency is kept in the narrowed project
		_, err = got.GetService("db")
		assert.NilError(t, err)
	})

	t.Run("a scheduled job without explicit manual opt-out can be run", func(t *testing.T) {
		got, err := materializeManualJob(base(), "backup")
		assert.NilError(t, err)
		svc, err := got.GetService("backup")
		assert.NilError(t, err)
		assert.Equal(t, svc.Image, "backup-tool")
	})

	t.Run("manual: false explicitly forbids manual execution", func(t *testing.T) {
		_, err := materializeManualJob(base(), "rotation")
		assert.Error(t, err, `job "rotation" is declared with manual: false, it cannot be run manually`)
	})
}
