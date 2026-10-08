//go:build e2e && !windows

/*
   Copyright 2026 Docker Compose CLI authors

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

// The scenarios below lock the observable behavior of the foreground
// (non-detached) `compose up`, whose create and start phases are driven by two
// separate code paths today. They exist so a change of the machinery behind
// it (epic #14081, lot 2) can be reviewed against a fixed specification.
// Most e2e coverage of the start phase runs detached; these do not.

func TestUpInteractiveHookLogs(t *testing.T) {
	NewScenario(t, "a foreground up must stream pre_start and post_start hook output into its session").
		Step("up prints both hooks' output, and the service reads what pre_start wrote before it started",
			ComposeCmd("up", "--menu=false").Within(120*time.Second),
			OutputContains("pre-start-hook-ran"),
			OutputContains("app-read:pre-start-hook-ran"),
			OutputContains("post-start-hook-ran"),
			ServiceState("app", "exited"))
}

func TestUpInteractiveImmediateExit(t *testing.T) {
	NewScenario(t, "a foreground up --abort-on-container-exit must notice a container that exits as soon as it starts").
		Step("up aborts on the instantly exiting container instead of waiting for the long-running one",
			ComposeCmd("up", "--menu=false", "--abort-on-container-exit").Within(60*time.Second),
			OutputContains("quick-1 exited with code 0"),
			OutputContains("Aborting on container exit"),
			ServiceState("quick", "exited"),
			ServiceState("long", "exited"))
}

func TestUpInteractiveRestartsExitedContainer(t *testing.T) {
	NewScenario(t, "a foreground up must start a container that exited earlier, reusing it").
		Step("up -d leaves the service exited after its first run",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "exited"), 15*time.Second)).
		Step("a foreground up runs the same container a second time",
			ComposeCmd("up", "--menu=false").Within(60*time.Second),
			NotRecreated("app"),
			OutputContains("second-run"),
			ServiceState("app", "exited"))
}

func TestUpInteractiveWaitsForHealthy(t *testing.T) {
	NewScenario(t, "a foreground up must not start a service before its service_healthy dependency is healthy").
		Step("up starts web only once db reports healthy",
			ComposeCmd("up", "--menu=false", "--abort-on-container-exit").Within(120*time.Second),
			OutputContains("web-saw-ready"),
			OutputNotContains("web-too-early"))
}

func TestUpInteractiveRecreateStreamsNewContainer(t *testing.T) {
	NewScenario(t, "a foreground up after a config change must recreate the service and stream the new container's output").
		Step("up -d creates the service with its first configuration",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "exited"), 15*time.Second)).
		Step("a foreground up with another value recreates it and shows the new output",
			ComposeCmd("up", "--menu=false").WithEnv("VALUE=two").Within(60*time.Second),
			Recreated("app"),
			OutputContains("value=two"))
}

func TestUpInteractiveDependencyTimeout(t *testing.T) {
	NewScenario(t, "a foreground up with --wait-timeout must fail once a dependency stays unhealthy, without starting the dependent").
		Step("up fails on the dependency timeout; web is created but never started",
			ComposeCmd("up", "--menu=false", "--wait-timeout", "3").MayFail().Within(60*time.Second),
			ExitCode(1),
			OutputContains("timeout waiting for dependencies"),
			OutputNotContains("web-started"),
			ServiceState("db", "running"),
			ServiceState("web", "created"))
}

func TestUpDryRunCreatesNothing(t *testing.T) {
	s := NewScenario(t, "a foreground up --dry-run must plan the create phase only, never the start phase")
	s.Step("up --dry-run reports the creation, no start, and leaves no container behind",
		ComposeCmd("up", "--menu=false", "--dry-run").Within(60*time.Second),
		OutputContains("Container "+s.Project()+"-app-1 Created"),
		OutputNotContains("Start"),
		OutputContains("interactive run is not supported in dry-run mode"),
		ServiceNotCreated("app"))
}
