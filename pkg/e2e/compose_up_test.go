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
)

func TestUpWait(t *testing.T) {
	s := NewScenario(t, "up --wait must return once dependencies completed and services run")
	s.Step("up --wait returns with the long-running service up and the oneshot completed",
		ComposeCmd("up", "--wait", "-d").Within(30*time.Second),
		OutputContains(s.Project()+"-oneshot-1"),
		ServiceState("longrunning", "running"),
		ServiceState("oneshot", "exited"))
}

// https://github.com/docker/compose/issues/9122: up --wait ran fully detached,
// leaving CI with no way to see why a service was slow or never got healthy.
func TestUpWaitStreamsLogs(t *testing.T) {
	NewScenario(t, "up --wait must stream the services' logs while waiting, then return leaving them running").
		Step("up --wait relays the service output and returns once it is healthy",
			ComposeCmd("up", "--wait").Within(60*time.Second),
			StdoutContains("booting"),
			StdoutContains("ready-to-serve"),
			ServiceHealthy("app"),
			ServiceState("app", "running"))
}

// Pins that the --wait readiness check runs on the caller's context: on the
// session's own it fails with "context canceled" once the followed service exits.
func TestUpWaitNoAttachOneShot(t *testing.T) {
	NewScenario(t, "up --wait must still succeed when the only service whose logs are followed exits during the wait").
		Step("up --wait returns once the long-running service is up, the followed one-shot having exited",
			ComposeCmd("up", "--wait", "--no-attach", "longrunning").Within(60*time.Second),
			ServiceState("longrunning", "running"),
			ServiceState("oneshot", "exited"))
}

func TestUpWaitDetachedStaysQuiet(t *testing.T) {
	NewScenario(t, "up --detach --wait must not stream the services' logs, as an explicit --detach asks for").
		Step("up -d --wait returns once the service is healthy without relaying its output",
			ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
			OutputNotContains("ready-to-serve"),
			ServiceHealthy("app"))
}

func TestUpWaitStreamsLogsOnUnhealthy(t *testing.T) {
	NewScenario(t, "up --wait must stream the logs of a service that never gets healthy, and still fail").
		Step("up --wait fails with the service output in the log",
			ComposeCmd("up", "--wait").MayFail().Within(60*time.Second),
			ExitCode(1),
			StdoutContains("failing-to-boot"),
			ServiceState("app", "running"))
}

func TestUpWaitTimeout(t *testing.T) {
	s := NewScenario(t, "up --wait --wait-timeout must fail once the timeout expires instead of silently succeeding")
	s.Step("up --wait fails after the timeout with the service still starting",
		ComposeCmd("up", "--wait", "--wait-timeout", "3", "-d").MayFail().Within(60*time.Second),
		ExitCode(1),
		OutputContains("application not healthy after 3s"),
		ServiceState("app", "running"))
}

// The dependency waits of the plan are bounded by --wait-timeout even without
// --wait, and an expired one is reported as a dependency timeout, not as the
// final "application not healthy" verification of --wait.
func TestUpWaitTimeoutOnDependency(t *testing.T) {
	NewScenario(t, "up --wait-timeout must bound the wait on a depends_on service_healthy dependency that never gets healthy").
		Step("up -d fails with a dependency timeout and never starts the dependent service",
			ComposeCmd("up", "-d", "--wait-timeout", "3").MayFail().Within(60*time.Second),
			ExitCode(1),
			OutputContains("timeout waiting for dependencies"),
			OutputNotContains("deadline exceeded"),
			ServiceState("db", "running"),
			ServiceState("web", "created"))
}

func TestUpExitCodeFrom(t *testing.T) {
	NewScenario(t, "up --exit-code-from must return the selected service's exit code").
		Step("up returns the failing service's code once it exits",
			ComposeCmd("up", "--menu=false", "--exit-code-from=failure", "failure").MayFail().Within(60*time.Second),
			ExitCode(42))
}

func TestUpExitCodeFromContainerKilled(t *testing.T) {
	NewScenario(t, "up --exit-code-from must report 143 for a service stopped by the abort").
		Step("the watched long-lived service is stopped when another exits",
			ComposeCmd("up", "--menu=false", "--exit-code-from=test").MayFail().Within(120*time.Second),
			ExitCode(143))
}

func TestPortRange(t *testing.T) {
	NewScenario(t, "a published port range must accommodate scaled replicas and single ports alike").
		Step("up binds every replica within the range",
			ComposeCmd("up", "-d"),
			ServiceScale("a", 5),
			ServiceState("b", "running"),
			ServiceState("c", "running"))
}

func TestStdoutStderr(t *testing.T) {
	NewScenario(t, "up must relay each container stream to its own: stdout to stdout, stderr to stderr").
		Step("the two streams arrive separated",
			ComposeCmd("up", "--menu=false"),
			StdoutContains("log to stdout"),
			StderrContains("log to stderr"))
}

func TestLoggingDriver(t *testing.T) {
	s := NewScenario(t, "a logging-driver address change must reconfigure the service on the next up")
	host := "127.0.0.1"
	if strings.Contains(s.CLI().RunDockerCmd(t, "info", "-f", "{{.OperatingSystem}}").Stdout(), "Docker Desktop") {
		host = "host.docker.internal"
	}
	s.Env("HOST="+host).
		Step("up starts the log collector and the app",
			ComposeCmd("up", "-d").WithEnv("BAR=foo"),
			ServiceState("fluentbit", "running"),
			ServiceState("app", "running")).
		Step("a collector config change is applied by recreating it",
			ComposeCmd("up", "-d").WithEnv("BAR=zot"),
			Recreated("fluentbit"),
			ServiceState("app", "running"))
}
