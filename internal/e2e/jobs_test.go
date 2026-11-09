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
	"strings"
	"testing"
	"time"

	extensionclient "github.com/moby/extensions/client"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	jobsv0 "github.com/docker/compose/v5/internal/jobsapi"
	jobspb "github.com/docker/compose/v5/internal/jobsapi/protogen"
)

// jobsAPIVersion is the first engine API version shipping the jobs
// extension, see apiVersionJobs in pkg/compose/api_versions.go, which it must
// follow. The extension is disabled by default, so this version does not mean
// jobs are usable. Below it, or with the extension disabled, Compose degrades:
// manual jobs run as one-off services and scheduled jobs are refused.
const jobsAPIVersion = "1.57"

// JobsExtensionEnabled requires the daemon's jobs extension to be enabled and
// ready: dockerd started with --extension-enable org.mobyproject.jobs.v0.
func JobsExtensionEnabled(t testing.TB, c *CLI) string {
	t.Helper()
	if !jobsExtensionActive(t, c) {
		return "the engine's jobs extension is not enabled"
	}
	return ""
}

// JobsExtensionDisabled requires the daemon to have no usable jobs extension:
// either its API predates jobs or the extension is not enabled.
func JobsExtensionDisabled(t testing.TB, c *CLI) string {
	t.Helper()
	if jobsExtensionActive(t, c) {
		return "the engine's jobs extension is enabled"
	}
	return ""
}

// jobsExtensionActive probes the daemon the CLI under test talks to, the way
// Compose does: the engine lists no extension, only the jobs service answering
// tells (Unimplemented when the extension is off). An unreachable or not ready
// extension fails the test rather than skipping it, so a misconfigured daemon
// never silently skips every jobs scenario.
func jobsExtensionActive(t testing.TB, c *CLI) bool {
	t.Helper()
	if versions.LessThan(engineAPIVersion(t, c), jobsAPIVersion) {
		return false
	}
	// The CLI resolves its daemon from DOCKER_HOST/DOCKER_CONTEXT; so does
	// `docker context inspect`, without a context name.
	host := strings.TrimSpace(c.RunDockerCmd(t, "context", "inspect", "-f", "{{.Endpoints.docker.Host}}").Stdout())
	apiClient, err := client.New(client.WithHost(host))
	if err != nil {
		t.Fatalf("connecting to the daemon at %s: %v", host, err)
	}
	defer apiClient.Close() //nolint:errcheck
	exts, err := extensionclient.New(apiClient, extensionclient.WithGRPCPoint(jobspb.ClientPoint))
	if err != nil {
		t.Fatalf("extension client: %v", err)
	}
	defer exts.Close() //nolint:errcheck
	jobs, err := extensionclient.Resolve(exts, jobsv0.Point)
	if err != nil {
		t.Fatalf("resolving the jobs extension: %v", err)
	}
	_, err = jobs.List(t.Context(), &jobsv0.ListRequest{})
	switch status.Code(err) {
	case codes.OK:
		return true
	case codes.Unimplemented:
		return false
	default:
		t.Fatalf("probing the jobs extension at %s: %v", host, err)
		return false
	}
}

// scheduledJobsRefused is the error of an engine without the jobs API refusing
// the project of TestUpRejectsScheduledJobs.
const scheduledJobsRefused = "scheduled jobs are not supported by this version of the engine: backup"

// scheduledJobsExtensionOffRefused is the error of an engine having the jobs
// API but not the extension enabled, refusing the same project.
const scheduledJobsExtensionOffRefused = "scheduled jobs need the engine's jobs extension, " +
	"which is not enabled (start dockerd with --extension-enable org.mobyproject.jobs.v0): backup"

// Without the jobs API, scheduled jobs cannot run: silently not scheduling
// them would break the user's expectations, so every command bringing the
// project up must refuse it as a whole, before creating anything.
func TestUpRejectsScheduledJobs(t *testing.T) {
	NewScenario(t, "up, create and start must reject a project declaring active scheduled jobs when the engine has no jobs API, before creating anything").
		Requires(EngineAPIVersionBelow(jobsAPIVersion)).
		Step("up fails naming the scheduled job",
			ComposeCmd("up", "-d").MayFail(),
			StderrContains(scheduledJobsRefused),
			ServiceNotCreated("web")).
		Step("create fails the same way",
			ComposeCmd("create").MayFail(),
			StderrContains(scheduledJobsRefused),
			ServiceNotCreated("web")).
		Step("start fails the same way",
			ComposeCmd("start").MayFail(),
			StderrContains(scheduledJobsRefused),
			ServiceNotCreated("web"))
}

// The jobs API alone does not make jobs usable: the engine ships the extension
// disabled by default, and an engine can only say so when asked. The refusal
// must be the same as on an engine without the API, and just as early, naming
// the dockerd flag that enables the extension.
func TestUpRejectsScheduledJobsWithExtensionOff(t *testing.T) {
	NewScenario(t, "up, create and start must reject a project declaring active scheduled jobs when the engine's jobs extension is not enabled, before creating anything").
		Requires(EngineAPIVersionAtLeast(jobsAPIVersion), JobsExtensionDisabled).
		Step("up fails naming the scheduled job and the flag enabling the extension",
			ComposeCmd("up", "-d").MayFail(),
			StderrContains(scheduledJobsExtensionOffRefused),
			ServiceNotCreated("web")).
		Step("create fails the same way",
			ComposeCmd("create").MayFail(),
			StderrContains(scheduledJobsExtensionOffRefused),
			ServiceNotCreated("web")).
		Step("start fails the same way",
			ComposeCmd("start").MayFail(),
			StderrContains(scheduledJobsExtensionOffRefused),
			ServiceNotCreated("web"))
}

// Without usable jobs support the engine has no job of the project to
// remove: down must not turn that into an error nor into noise.
func TestDownWithoutJobsExtension(t *testing.T) {
	NewScenario(t, "down must clean up the project without a word about jobs when the engine cannot run jobs").
		Requires(JobsExtensionDisabled).
		Step("up starts the service, the manual job waits for run",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running")).
		Step("down removes the service and ignores the jobs silently",
			ComposeCmd("down"),
			ServiceNotCreated("web"),
			OutputNotContains("Job "),
			OutputNotContains("level=warning"))
}

// A scheduled job registers with the engine instead of being rejected: up is
// safely re-runnable on an unchanged spec, and the schedule fires on the
// engine's own clock, independent of the client.
func TestUpRegistersScheduledJobs(t *testing.T) {
	NewScenario(t, "up must register a project's scheduled jobs with the engine and let them fire on their own").
		Requires(JobsExtensionEnabled).
		Step("up starts services and registers the scheduled job",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running")).
		Step("re-up is a no-op on the unchanged job spec, and the schedule fires on the engine's own clock",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running"),
			Eventually(ServiceState("backup", "exited"), 90*time.Second))
}

// up --no-start stops after creating the containers, and must still register
// the scheduled jobs: the engine's cron does not wait for a Start.
func TestUpNoStartRegistersScheduledJobs(t *testing.T) {
	NewScenario(t, "up --no-start must still register a project's scheduled jobs with the engine").
		Requires(JobsExtensionEnabled).
		Step("up --no-start creates but never starts web, yet the schedule still fires on its own",
			ComposeCmd("up", "--no-start"),
			ServiceState("web", "created"),
			Eventually(ServiceState("backup", "exited"), 90*time.Second))
}

// A project with no service has nothing to follow: an attached up registers
// the jobs and returns, even if an earlier run left its container behind.
func TestUpWithOnlyJobsReturns(t *testing.T) {
	NewScenario(t, "up must register the scheduled jobs of a project without service and return").
		Requires(JobsExtensionEnabled).
		Step("up registers the scheduled job",
			ComposeCmd("up", "-d")).
		Step("a failed run leaves its container behind",
			ComposeCmd("run", "cron").MayFail()).
		// an up that attaches to that container never returns: Within kills it and
		// fails the step
		Step("an attached up does not wait for, or attach to, that container",
			ComposeCmd("up").Within(20*time.Second),
			OutputNotContains("Attaching to"))
}

// create arms the schedule like up does, whether or not the services are
// started: a run that fails for want of them leaves its logs to read.
func TestCreateRegistersScheduledJobs(t *testing.T) {
	NewScenario(t, "create must register a project's scheduled jobs with the engine").
		Requires(JobsExtensionEnabled).
		Step("create makes web without starting it, yet the schedule fires on its own",
			ComposeCmd("create"),
			ServiceState("web", "created"),
			Eventually(ServiceState("backup", "exited"), 90*time.Second))
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
