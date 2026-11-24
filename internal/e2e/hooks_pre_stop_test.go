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

// A pre_stop hook runs a command in the process of a container. These scenarios
// lock what happens to the stop and the restart of a container that has none
// (docker/compose#14545): the hooks are skipped, and the engine does the rest.
// Each hook appends a line to a log in the project's data volume, so that what
// ran is read from state, not from what the CLI printed.

func TestRestartStoppedServiceWithPreStopHook(t *testing.T) {
	s := NewScenario(t, "restart must bring a stopped service back up without running its pre_stop hook")
	s.Step("up starts both services",
		ComposeCmd("up", "-d"),
		ServiceState("hooked", "running"),
		ServiceState("plain", "running")).
		Step("stop runs the hook in the running container, then halts both",
			ComposeCmd("stop", "-t", "1"),
			ServiceState("hooked", "exited"),
			ServiceState("plain", "exited")).
		Step("restart starts both again, instead of failing on the hook of the stopped service",
			ComposeCmd("restart"),
			Eventually(ServiceState("hooked", "running"), 10*time.Second),
			Eventually(ServiceState("plain", "running"), 10*time.Second),
			NotRecreated("hooked", "plain")).
		Step("the hook ran once, for the stop, and not for the restart",
			probeVolume(s, "cat", "/mnt/log.txt"),
			OutputMatchesCount("pre_stop ran", 1))
}

func TestRestartRunningServiceRunsPreStopHook(t *testing.T) {
	s := NewScenario(t, "restart must still run the pre_stop hook of a running container")
	s.Step("up starts both services",
		ComposeCmd("up", "-d"),
		ServiceState("hooked", "running"),
		ServiceState("plain", "running")).
		Step("restart runs the hook, then restarts both in place",
			ComposeCmd("restart", "-t", "1"),
			Eventually(ServiceState("hooked", "running"), 10*time.Second),
			Eventually(ServiceState("plain", "running"), 10*time.Second),
			NotRecreated("hooked", "plain")).
		Step("the hook ran once",
			probeVolume(s, "cat", "/mnt/log.txt"),
			OutputMatchesCount("pre_stop ran", 1))
}

func TestStopPausedServiceWithPreStopHook(t *testing.T) {
	s := NewScenario(t, "stop must halt a paused service, whose pre_stop hook cannot run, instead of skipping it")
	s.Step("up starts both services",
		ComposeCmd("up", "-d"),
		ServiceState("hooked", "running"),
		ServiceState("plain", "running")).
		Step("pause freezes both",
			ComposeCmd("pause"),
			ServiceState("hooked", "paused"),
			ServiceState("plain", "paused")).
		Step("stop halts both, the hooked one included",
			ComposeCmd("stop", "-t", "1"),
			ServiceState("hooked", "exited"),
			ServiceState("plain", "exited")).
		Step("the hook never ran: there was no process for it",
			probeVolume(s, "ls", "/mnt"),
			OutputNotContains("log.txt"))
}

func TestDownPausedServiceWithPreStopHook(t *testing.T) {
	s := NewScenario(t, "down must remove a paused service whose pre_stop hook cannot run")
	s.Step("up starts both services",
		ComposeCmd("up", "-d"),
		ServiceState("hooked", "running"),
		ServiceState("plain", "running")).
		Step("pause freezes both",
			ComposeCmd("pause"),
			ServiceState("hooked", "paused"),
			ServiceState("plain", "paused")).
		Step("down removes both",
			ComposeCmd("down", "-t", "1"),
			ServiceNotCreated("hooked"),
			ServiceNotCreated("plain"))
}
