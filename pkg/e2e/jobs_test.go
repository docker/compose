//go:build e2e

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

package e2e

import (
	"testing"
)

// Scheduled jobs cannot run in this version: silently not scheduling them
// would break the user's expectations, so up must refuse the whole project.
func TestUpRejectsScheduledJobs(t *testing.T) {
	NewScenario(t, "up must reject a project declaring active scheduled jobs, before creating anything").
		Step("up fails naming the scheduled job",
			ComposeCmd("up", "-d").MayFail(),
			StderrContains("scheduled jobs are not supported in this version: backup"),
			ServiceNotCreated("web"))
}

// A job runs through `compose run` exactly like a service would: its
// declared dependencies start first, its output and exit flow back. Per the
// spec, manual execution is always available — scheduled jobs included —
// unless the job explicitly opts out with `triggers.manual: false`.
func TestRunManualJob(t *testing.T) {
	NewScenario(t, "run must execute a job like a service, starting its depends_on services first").
		Step("run executes the job after starting its dependency",
			ComposeCmd("run", "--rm", "migrate"),
			OutputContains("migration done"),
			ServiceState("db", "running")).
		Step("a scheduled job without explicit opt-out can be run manually",
			ComposeCmd("run", "--rm", "backup"),
			OutputContains("backup")).
		Step("manual: false explicitly forbids manual execution",
			ComposeCmd("run", "--rm", "rotation").MayFail(),
			StderrContains(`job "rotation" is declared with manual: false`))
}

// A job's own env_file resolves exactly like a selected service's would: the
// materialization happens before environment resolution, not after.
func TestRunJobEnvFile(t *testing.T) {
	NewScenario(t, "a job's env_file must feed its environment through run").
		Step("the job sees the env_file variable",
			ComposeCmd("run", "--rm", "migrate"),
			OutputContains("DB_URL=postgres://db:5432/app"))
}

// A job may depend on another job: the dependency job runs to completion
// first — through the exact machinery a service dependency does — then the
// target runs.
func TestRunJobDependsOnJob(t *testing.T) {
	NewScenario(t, "a job depending on a job must run the dependency to completion first").
		Step("the dependency job completes before the target runs",
			ComposeCmd("run", "--rm", "deploy"),
			OutputContains("deploy done"),
			ServiceState("db", "running"))
}

// manual: false declares a job harmful to trigger outside its schedule.
// depends_on doesn't change who caused the execution or when: pulling the
// job in as a dependency of a manually-run job is still the run command
// causing that out-of-schedule execution, one hop removed, so it must be
// refused too — before anything else in the closure is created.
func TestRunJobDependsOnManualFalseJob(t *testing.T) {
	NewScenario(t, "a job depending on a manual: false job must refuse to run, before creating anything").
		Step("run fails naming the manual: false dependency",
			ComposeCmd("run", "--rm", "deploy").MayFail(),
			StderrContains(`job "sensitive" is declared with manual: false, it cannot be triggered even as a dependency`),
			ServiceNotCreated("db"))
}

// A job is documented as run-only: create and start don't know how to
// materialize one, so targeting either by a job's name must say so clearly
// instead of surfacing compose-go's raw "no such service" selection error.
// The same refusal is shared by every other service-targeting command
// through projectOrName -- see TestStopRefusesJob and
// TestDownRefusesJobWithProjectNameEnv below.
func TestCreateRefusesJob(t *testing.T) {
	NewScenario(t, "create must refuse a job by name, naming run as the right command").
		Step("create fails naming the job",
			ComposeCmd("create", "migrate").MayFail(),
			StderrContains(`job "migrate" can only be triggered with "docker compose run"`),
			ServiceNotCreated("migrate"))
}

func TestStartRefusesJob(t *testing.T) {
	NewScenario(t, "start must refuse a job by name, naming run as the right command").
		Step("start fails naming the job",
			ComposeCmd("start", "migrate").MayFail(),
			StderrContains(`job "migrate" can only be triggered with "docker compose run"`),
			ServiceNotCreated("migrate"))
}

// When COMPOSE_PROJECT_NAME is set, projectOrName falls back to a
// label-driven, file-less project on any load failure -- including "no
// such service" for a job -- instead of surfacing it. Without an explicit
// job check on that path, start would exit 0 having silently done
// nothing: a job that was never run has no container for the
// label-driven fallback to find.
func TestStartRefusesJobWithProjectNameEnv(t *testing.T) {
	s := NewScenario(t, "start must still refuse a job by name when COMPOSE_PROJECT_NAME triggers the label-driven fallback")
	s.Step("start fails naming the job, not silently exiting 0",
		ComposeCmd("start", "migrate").WithEnv("COMPOSE_PROJECT_NAME="+s.Project()).MayFail(),
		StderrContains(`job "migrate" can only be triggered with "docker compose run"`),
		ServiceNotCreated("migrate"))
}

// projectOrName is shared by every service-targeting command besides
// run/create/start (stop, kill, pause/unpause, logs, rm, down, ps,
// events): the same job refusal applies to all of them. stop stands in
// for that whole family here.
func TestStopRefusesJob(t *testing.T) {
	NewScenario(t, "stop must refuse a job by name, naming run as the right command").
		Step("stop fails naming the job",
			ComposeCmd("stop", "migrate").MayFail(),
			StderrContains(`job "migrate" can only be triggered with "docker compose run"`),
			ServiceNotCreated("migrate"))
}

// Same COMPOSE_PROJECT_NAME fallback as TestStartRefusesJobWithProjectNameEnv,
// exercised through a second projectOrName caller (down) to confirm the fix
// lives in the shared helper, not duplicated per command.
func TestDownRefusesJobWithProjectNameEnv(t *testing.T) {
	s := NewScenario(t, "down must still refuse a job by name when COMPOSE_PROJECT_NAME triggers the label-driven fallback")
	s.Step("down fails naming the job, not silently exiting 0",
		ComposeCmd("down", "migrate").WithEnv("COMPOSE_PROJECT_NAME="+s.Project()).MayFail(),
		StderrContains(`job "migrate" can only be triggered with "docker compose run"`),
		ServiceNotCreated("migrate"))
}
