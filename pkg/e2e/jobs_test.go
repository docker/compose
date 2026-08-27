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
	"time"
)

// A scheduled job registers with the engine instead of being rejected: up is
// safely re-runnable on an unchanged spec, and the schedule fires on the
// engine's own clock, independent of the client.
func TestUpRegistersScheduledJobs(t *testing.T) {
	NewScenario(t, "up must register a project's scheduled jobs with the engine and let them fire on their own").
		Step("up starts services and registers the scheduled job",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running")).
		Step("re-up is a no-op on the unchanged job spec, and the schedule fires on the engine's own clock",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running"),
			Eventually(ServiceState("backup", "exited"), 90*time.Second))
}

// A manual-trigger job runs through `compose run` exactly like a service
// would: its declared dependencies start first, its output and exit flow
// back. A schedule-only job stays out of reach.
func TestRunManualJob(t *testing.T) {
	NewScenario(t, "run must execute a manual job like a service, starting its depends_on services first").
		Step("run executes the job after starting its dependency",
			ComposeCmd("run", "--rm", "migrate"),
			OutputContains("migration done"),
			ServiceState("db", "running")).
		Step("a schedule-only job cannot be run",
			ComposeCmd("run", "--rm", "backup").MayFail(),
			StderrContains(`job "backup" has no manual trigger`))
}
