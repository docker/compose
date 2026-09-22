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

func TestHasSchedule(t *testing.T) {
	assert.Assert(t, !HasSchedule(types.JobConfig{}))
	assert.Assert(t, !HasSchedule(types.JobConfig{Triggers: &types.TriggerConfig{}}))
	assert.Assert(t, HasSchedule(types.JobConfig{Triggers: &types.TriggerConfig{
		Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}},
	}}))
}

func TestManualTriggerDisabled(t *testing.T) {
	yes, no := true, false

	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{}), "no triggers at all is not an opt-out")
	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{}}), "an unset Manual is not an opt-out")
	assert.Assert(t, !ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{Manual: &yes}}))
	assert.Assert(t, ManualTriggerDisabled(types.JobConfig{Triggers: &types.TriggerConfig{Manual: &no}}))
}

func TestSortedJobNames(t *testing.T) {
	jobs := types.Jobs{
		"backup":  {Triggers: &types.TriggerConfig{Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}}},
		"migrate": {},
		"report":  {},
	}

	assert.DeepEqual(t, sortedJobNames(jobs, nil), []string{"backup", "migrate", "report"})
	assert.DeepEqual(t, sortedJobNames(jobs, HasSchedule), []string{"backup"})
	assert.DeepEqual(t, sortedJobNames(types.Jobs{}, HasSchedule), []string{})
}

func TestJobChangedErr(t *testing.T) {
	assert.Error(t, jobChangedErr("backup", "up"),
		`job "backup" has changed: run `+"`docker compose down`"+` to remove it, then `+"`up`"+` again`)
	assert.Error(t, jobChangedErr("migrate", "run"),
		`job "migrate" has changed: run `+"`docker compose down`"+` to remove it, then `+"`run`"+` again`)
}

func TestManualTriggerDisabledErr(t *testing.T) {
	assert.Error(t, ManualTriggerDisabledErr("rotation"),
		`job "rotation" is declared with manual: false, it cannot be run manually`)
}
