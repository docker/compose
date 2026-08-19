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
