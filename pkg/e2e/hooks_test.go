//go:build e2e

/*
Copyright 2023 Docker Compose CLI authors

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

// probeVolume materializes the "read a file from the project's data volume"
// action the pre_start scenarios use to observe what the hooks wrote.
func probeVolume(s *Scenario, args ...string) Action {
	return DockerCmd(append([]string{"run", "--rm", "-v", s.Project() + "_data:/mnt", "alpine"}, args...)...)
}

func TestPostStartHookInError(t *testing.T) {
	NewScenario(t, "a failing post_start hook must fail up, reporting the hook's exit status").
		Step("up fails on the hook error",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			OutputContains("test hook exited with status 127"))
}

func TestPostStartHookSuccess(t *testing.T) {
	NewScenario(t, "a post_start hook must run after the service starts, without failing up").
		Step("up runs the hook and leaves the service running",
			ComposeCmd("up", "-d"),
			ServiceState("test", "running"))
}

func TestPreStopHookSuccess(t *testing.T) {
	s := NewScenario(t, "a pre_stop hook must run in the container before it is stopped")
	s.Step("up starts the service",
		ComposeCmd("up", "-d"),
		ServiceState("sample", "running")).
		Step("stop runs the hook before halting the container",
			ComposeCmd("stop"),
			ServiceState("sample", "exited")).
		Step("the hook's write is visible in the volume",
			probeVolume(s, "cat", "/mnt/log.txt"),
			OutputContains("In the pre-stop"))
}

func TestPreStopHookInError(t *testing.T) {
	NewScenario(t, "a failing pre_stop hook must fail the down, reporting the hook's exit status").
		Step("up starts the service",
			ComposeCmd("up", "-d"),
			ServiceState("sample", "running")).
		Step("down fails on the hook error",
			ComposeCmd("down", "-t", "0").MayFail(),
			ExitCode(1),
			OutputContains("sample hook exited with status 127"))
}

func TestPreStopHookSuccessWithPreviousStop(t *testing.T) {
	NewScenario(t, "stopping a single service must run its pre_stop hook and only halt that service").
		Step("up starts both services",
			ComposeCmd("up", "-d"),
			ServiceState("sample", "running"),
			ServiceState("test", "running")).
		Step("stop on the hooked service succeeds",
			ComposeCmd("stop", "sample"),
			ServiceState("sample", "exited"),
			ServiceState("test", "running"))
}

func TestPostStartAndPreStopHook(t *testing.T) {
	NewScenario(t, "post_start and pre_stop hooks in one project must not interfere with up").
		Step("up starts both services and runs the post_start hook",
			ComposeCmd("up", "-d"),
			ServiceState("sample", "running"),
			ServiceState("test", "running"))
}

func TestPreStartHookSuccess(t *testing.T) {
	NewScenario(t, "a pre_start hook must run before the service, which reads what the hook prepared").
		Step("up waits for the service, started after the hook",
			ComposeCmd("up", "-d", "--wait").Within(60*time.Second)).
		Step("the service saw the file the hook wrote",
			ComposeCmd("logs", "sample"),
			OutputContains("initialized"))
}

func TestPreStartHookInError(t *testing.T) {
	NewScenario(t, "a failing pre_start hook must fail up and leave the service created but not started").
		Step("up fails, reporting the hook's exit code",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			OutputContains("pre_start"),
			OutputContains("17"),
			ServiceState("sample", "created"))
}

func TestPreStartHookBuildInheritance(t *testing.T) {
	s := NewScenario(t, "a pre_start hook without an image must run on the service's built image")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-sample")).
		Step("up builds the image and runs the hook on it",
			ComposeCmd("up", "-d", "--wait").Within(120*time.Second)).
		Step("the service saw the marker only the built image could produce",
			ComposeCmd("logs", "sample"),
			OutputContains("built-image-marker"))
}

func TestPreStartHookIdempotentReUp(t *testing.T) {
	s := NewScenario(t, "an unchanged up must not re-run pre_start hooks on a running service")
	s.Step("first up runs the hook once",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second)).
		Step("the hook wrote exactly one token",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("1 /mnt/tokens.txt")).
		Step("an unchanged up does not re-run the hook",
			ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
			NotRecreated("sample")).
		Step("still exactly one token",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("1 /mnt/tokens.txt"))
}

func TestPreStartHookReRunOnSpecChange(t *testing.T) {
	s := NewScenario(t, "a spec change must recreate the service and re-run its pre_start hooks")
	s.Step("up with the v1 spec runs the v1 hook",
		ComposeCmd("up", "-d", "--wait").WithEnv("HOOK_VERSION=v1").Within(60*time.Second)).
		Step("the volume records v1 only",
			probeVolume(s, "cat", "/mnt/versions.txt"),
			OutputContains("v1"),
			OutputNotContains("v2")).
		Step("up with the v2 spec recreates the service and re-runs the hook",
			ComposeCmd("up", "-d", "--wait").WithEnv("HOOK_VERSION=v2").Within(60*time.Second),
			Recreated("sample")).
		Step("the volume records both versions",
			probeVolume(s, "cat", "/mnt/versions.txt"),
			OutputContains("v1"),
			OutputContains("v2"))
}

func TestPreStartHookForceRecreate(t *testing.T) {
	s := NewScenario(t, "up --force-recreate must re-run pre_start hooks")
	s.Step("first up runs the hook once",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second)).
		Step("one token after the first up",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("1 /mnt/tokens.txt")).
		Step("force-recreate replaces the container and re-runs the hook",
			ComposeCmd("up", "-d", "--force-recreate", "--wait").Within(60*time.Second),
			Recreated("sample")).
		Step("two tokens after the recreate",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("2 /mnt/tokens.txt"))
}

func TestPreStartHookMidSequenceFailure(t *testing.T) {
	s := NewScenario(t, "a mid-sequence pre_start failure must run earlier hooks, then fail up pointing at the culprit")
	s.Step("up fails on the second hook, naming its index and exit code",
		ComposeCmd("up", "-d").MayFail(),
		ExitCode(1),
		OutputContains("pre_start[1]"),
		OutputContains("17"),
		ServiceState("sample", "created")).
		Step("the first hook did run before the failure",
			probeVolume(s, "cat", "/mnt/hooks.txt"),
			OutputContains("ran-0"))
}

func TestPreStartHookSequentialOrder(t *testing.T) {
	s := NewScenario(t, "pre_start hooks must run sequentially, in declaration order")
	s.Step("up runs both hooks",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second)).
		Step("the volume shows A before B",
			probeVolume(s, "cat", "/mnt/out"),
			OutputContains("A\nB"))
}

func TestPreStartHookNotReRunOnScaleUp(t *testing.T) {
	s := NewScenario(t, "scaling up must not re-run pre_start hooks while a replica is already running")
	s.Step("first up runs the hook once",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second)).
		Step("one token after the first up",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("1 /mnt/tokens.txt")).
		Step("scaling up adds a replica without re-running the hook",
			ComposeCmd("up", "-d", "--scale", "sample=2", "--wait").Within(60*time.Second),
			ServiceScale("sample", 2)).
		Step("still exactly one token",
			probeVolume(s, "wc", "-l", "/mnt/tokens.txt"),
			OutputContains("1 /mnt/tokens.txt"))
}

func TestPreStartHookRunsOnceForScaledService(t *testing.T) {
	s := NewScenario(t, "with the default per_replica: false, a pre_start hook must run once for the whole service")
	s.Step("up starts both replicas",
		ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
		ServiceScale("sample", 2)).
		Step("the hook ran exactly once across replicas",
			probeVolume(s, "wc", "-l", "/mnt/log"),
			OutputContains("1 /mnt/log"))
}
