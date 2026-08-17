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

func TestStartInterval(t *testing.T) {
	NewScenario(t, "start_interval must require start_period, and must accelerate the initial health probes").
		Step("up --wait on a start_interval without start_period is rejected",
			ComposeCmd("up", "--wait", "-d", "error").MayFail(),
			ExitCode(1),
			OutputContains("healthcheck.start_interval requires healthcheck.start_period to be set")).
		Step("up --wait turns healthy well before the regular 30s interval",
			ComposeCmd("up", "--wait", "-d", "test").Within(30*time.Second),
			ServiceState("test", "running"),
			ServiceHealthy("test"))
}
