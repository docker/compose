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

// pre_stop hooks run whenever Compose stops a container, not only for `stop`,
// `restart` and `down`: the stops that `up` makes to replace a container or to
// scale a service down go through the reconciliation plan, which did not run
// them. The hook of these projects appends a line to a log in the data volume,
// so what the scenarios observe is how many times it ran.

// hookRuns reads what the hook of the project wrote to its data volume.
func hookRuns(s *Scenario) Action {
	return probeVolume(s, "sh", "-c", "cat /mnt/log.txt 2>/dev/null; true")
}

func TestUpRunsPreStopHookOfTheContainerItRecreates(t *testing.T) {
	s := NewScenario(t, "up must run the pre_stop hook of a container it replaces, before stopping it")
	s.Step("up starts the service, whose hook has not run",
		ComposeCmd("up", "-d"),
		ServiceState("sample", "running")).
		Step("the hook has not run yet",
			hookRuns(s),
			OutputNotContains("pre_stop ran")).
		Step("up with a changed service replaces the container",
			ComposeCmd("up", "-d").WithEnv("VERSION=2"),
			Recreated("sample"),
			ServiceState("sample", "running")).
		Step("the hook ran once, in the container that went",
			hookRuns(s),
			OutputMatchesCount("pre_stop ran", 1))
}

func TestUpRunsPreStopHookOfTheReplicaItScalesDown(t *testing.T) {
	s := NewScenario(t, "up must run the pre_stop hook of the replicas a scale down removes, and of no other")
	s.Step("up starts two replicas",
		ComposeCmd("up", "-d", "--scale", "sample=2"),
		ServiceScale("sample", 2),
		ServiceState("sample", "running")).
		Step("up with one replica removes the second",
			ComposeCmd("up", "-d", "--scale", "sample=1"),
			ReplicaNumbers("sample", 1),
			ServiceState("sample", "running")).
		Step("the hook ran once, for the replica that went",
			hookRuns(s),
			OutputMatchesCount("pre_stop ran", 1))
}

// A stopped container has no process to run the hook in: the engine refuses the
// exec, as it did for `restart` and `stop` (#14545). Replacing it must neither
// fail nor pretend the hook ran.
func TestUpSkipsPreStopHookOfAStoppedContainerItRecreates(t *testing.T) {
	s := NewScenario(t, "up must replace a stopped container whose service has a pre_stop hook, without running the hook again")
	s.Step("up starts the service",
		ComposeCmd("up", "-d"),
		ServiceState("sample", "running")).
		Step("stop runs the hook and halts the container",
			ComposeCmd("stop"),
			ServiceState("sample", "exited")).
		Step("up with a changed service replaces the stopped container",
			ComposeCmd("up", "-d").WithEnv("VERSION=2"),
			Recreated("sample"),
			ServiceState("sample", "running")).
		Step("the hook ran once, when the service was stopped, and not for the replacement of the stopped container",
			hookRuns(s),
			OutputMatchesCount("pre_stop ran", 1))
}

func TestUpKeepsTheContainerWhoseFailingPreStopHookBlocksItsReplacement(t *testing.T) {
	NewScenario(t, "up must leave the service as it was when the pre_stop hook of the container it replaces fails, and replace it once the hook is fixed").
		Step("up starts the service",
			ComposeCmd("up", "-d"),
			ServiceState("sample", "running")).
		Step("up with a changed service fails on the hook, reporting its exit status",
			ComposeCmd("up", "-d").WithEnv("VERSION=2").MayFail().Within(60*time.Second),
			ExitCode(1),
			OutputContains("sample hook exited with status 3"),
			RunningContainersKept("sample")).
		Step("no replacement was left behind, for the next up to trip on",
			ComposeCmd("ps", "--all"),
			ServiceScale("sample", 1),
			ServiceState("sample", "running")).
		Step("up replaces the container once the hook is fixed",
			ComposeCmd("up", "-d").WithEnv("VERSION=2", "HOOK_EXIT=0").Within(60*time.Second),
			Recreated("sample"),
			ServiceScale("sample", 1),
			ServiceState("sample", "running"))
}
