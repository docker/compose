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

func TestUpExitCodeFrom(t *testing.T) {
	NewScenario(t, "up --exit-code-from must return the selected service's exit code").
		Step("up returns the failing service's code once it exits",
			ComposeCmd("up", "--menu=false", "--exit-code-from=failure", "failure").MayFail().Within(60*time.Second),
			ExitCode(42))
}

func TestUpExitCodeFromContainerKilled(t *testing.T) {
	NewScenario(t, "up --exit-code-from must report 143 for a service stopped by the abort").
		Step("the watched long-lived service is stopped when another exits",
			ComposeCmd("up", "--menu=false", "--exit-code-from=test").MayFail().Within(60*time.Second),
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
