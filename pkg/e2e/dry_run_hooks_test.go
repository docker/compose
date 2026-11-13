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

// A dry-run client cannot attach to an exec, so a lifecycle hook used to make
// every dry-run that reached one fail with "interactive exec is not supported
// in dry-run mode". The hooks must be skipped: no failure, no side effect.
func TestDryRunSkipsHooks(t *testing.T) {
	preStopNotRun := ExecOutputContains("app", "test -e /tmp/pre_stop_ran || echo not-run", "not-run")

	s := NewScenario(t, "a dry-run must neither fail on lifecycle hooks nor run them")
	s.Step("up starts the service and runs its post_start hook",
		ComposeCmd("up", "-d"),
		ServiceState("app", "running"),
		ExecOutputContains("app", "ls /tmp", "post_start_ran")).
		Step("a dry-run recreate leaves the container untouched and skips pre_stop",
			ComposeCmd("--dry-run", "up", "-d", "--force-recreate"),
			NotRecreated("app"),
			ServiceState("app", "running"),
			preStopNotRun).
		Step("a dry-run restart leaves the container untouched and skips pre_stop",
			ComposeCmd("--dry-run", "restart"),
			NotRecreated("app"),
			ServiceState("app", "running"),
			preStopNotRun).
		Step("a dry-run stop leaves the container running and skips pre_stop",
			ComposeCmd("--dry-run", "stop"),
			NotRecreated("app"),
			ServiceState("app", "running"),
			preStopNotRun).
		Step("a dry-run down -v leaves the container and its volume and skips pre_stop",
			ComposeCmd("--dry-run", "down", "-v"),
			NotRecreated("app"),
			ServiceState("app", "running"),
			VolumeExists(s.Project()+"_data"),
			preStopNotRun)
}
