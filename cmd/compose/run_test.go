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
	"errors"
	"os"
	"path/filepath"
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
				"prep": {
					Name:          "prep",
					Triggers:      &types.TriggerConfig{Manual: &yes},
					ContainerSpec: types.ContainerSpec{Image: "prep-tool"},
					WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"db": {Condition: types.ServiceConditionStarted, Required: true}}},
				},
				"deploy": {
					Name:          "deploy",
					Triggers:      &types.TriggerConfig{Manual: &yes},
					Extensions:    types.Extensions{"x-team": "platform"},
					ContainerSpec: types.ContainerSpec{Image: "deployer"},
					WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"prep": {Condition: types.ServiceConditionCompletedSuccessfully, Required: true}}},
				},
				"sensitive": {
					Name:     "sensitive",
					Triggers: &types.TriggerConfig{Manual: &no, Schedule: []types.ScheduleConfig{{Cron: "0 3 * * *"}}},
				},
				"escalate": {
					Name:          "escalate",
					Triggers:      &types.TriggerConfig{Manual: &yes},
					ContainerSpec: types.ContainerSpec{Image: "escalator"},
					WorkloadSpec:  types.WorkloadSpec{DependsOn: types.DependsOnConfig{"sensitive": {Condition: types.ServiceConditionCompletedSuccessfully, Required: true}}},
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

	t.Run("a job dependency on another job materializes the whole closure", func(t *testing.T) {
		got, err := materializeManualJob(base(), "deploy")
		assert.NilError(t, err)
		// deploy -> job prep -> service db: prep becomes a runnable service
		// so the depends_on edge resolves, and db is kept for prep
		prep, err := got.GetService("prep")
		assert.NilError(t, err)
		assert.Equal(t, prep.Image, "prep-tool")
		_, err = got.GetService("db")
		assert.NilError(t, err)
		deploy, err := got.GetService("deploy")
		assert.NilError(t, err)
		assert.Equal(t, deploy.Extensions["x-team"], "platform", "job extensions survive materialization")
	})

	t.Run("an existing service with the target name wins over a job", func(t *testing.T) {
		p := base()
		p.Services["migrate"] = types.ServiceConfig{Name: "migrate", ContainerSpec: types.ContainerSpec{Image: "the-service"}}
		got, err := materializeManualJob(p, "migrate")
		assert.NilError(t, err)
		svc, err := got.GetService("migrate")
		assert.NilError(t, err)
		assert.Equal(t, svc.Image, "the-service", "a same-named service must not be shadowed by the job")
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

	// depends_on doesn't change who caused the execution or when: pulling in
	// a manual: false job as a dependency of a manually-run job is still the
	// run command triggering it out of schedule, one hop removed.
	t.Run("manual: false also blocks the job when pulled in transitively", func(t *testing.T) {
		_, err := materializeManualJob(base(), "escalate")
		assert.Error(t, err, `job "sensitive" is declared with manual: false, it cannot be triggered even as a dependency of another job`)
	})
}

// runProject only retries the unselected/full load to look for a job when
// the narrowed load failed specifically because the target isn't a known
// service — any other load error (a bad include:, an interpolation
// error, ...) must not trigger that retry, since it would only duplicate
// side effects (remote include: fetches, unsupported-attribute warnings)
// before falling through to the same, unrecoverable error anyway.
func TestIsNoSuchServiceErr(t *testing.T) {
	assert.Assert(t, isNoSuchServiceErr(errors.New("no such service: migrate")))
	assert.Assert(t, !isNoSuchServiceErr(errors.New("interpolation error: bad substitution")))
	assert.Assert(t, !isNoSuchServiceErr(errors.New("include: remote resource fetch failed")))
}

// jobTargetErrFixture writes a project declaring one service and one job,
// for jobTargetErr to reload unselected against.
func jobTargetErrFixture(t *testing.T) *ProjectOptions {
	t.Helper()
	path := filepath.Join(t.TempDir(), "compose.yaml")
	content := `
name: test
services:
  web:
    image: alpine
jobs:
  migrate:
    image: alpine
    triggers:
      manual: true
`
	assert.NilError(t, os.WriteFile(path, []byte(content), 0o644))
	return &ProjectOptions{ConfigPaths: []string{path}}
}

func TestJobTargetErr(t *testing.T) {
	opts := jobTargetErrFixture(t)

	t.Run("a job name in the error is replaced with a clear message", func(t *testing.T) {
		err, replaced := jobTargetErr(t.Context(), nil, opts, []string{"migrate"}, errors.New("no such service: migrate"))
		assert.Assert(t, replaced)
		assert.Error(t, err, `job "migrate" can only be triggered with "docker compose run"`)
	})

	t.Run("a real typo among several targets keeps its own error, not a same-invocation job's", func(t *testing.T) {
		// "migrate" is a declared job and present in names, but the error
		// names "typo" -- the actual selection failure -- not "migrate":
		// only "typo" may be reported on, and it isn't a job, so the
		// original error must survive unreplaced.
		original := errors.New("no such service: typo")
		err, replaced := jobTargetErr(t.Context(), nil, opts, []string{"typo", "migrate"}, original)
		assert.Assert(t, !replaced)
		assert.Equal(t, err, original)
	})

	t.Run("the job is still recognized regardless of its position in names", func(t *testing.T) {
		err, replaced := jobTargetErr(t.Context(), nil, opts, []string{"web", "migrate"}, errors.New("no such service: migrate"))
		assert.Assert(t, replaced)
		assert.Error(t, err, `job "migrate" can only be triggered with "docker compose run"`)
	})

	t.Run("a non-job, non-selection error is returned unchanged", func(t *testing.T) {
		original := errors.New("interpolation error: bad substitution")
		err, replaced := jobTargetErr(t.Context(), nil, opts, []string{"migrate"}, original)
		assert.Assert(t, !replaced)
		assert.Equal(t, err, original)
	})
}
