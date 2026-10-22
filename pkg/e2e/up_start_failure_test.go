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

import "testing"

// TestUpStartFailureReportsInterruptedServices locks docker/compose#14372: when
// a service fails to start, the services the command could not start because of
// it are reported as such. A service held by a depends_on wait, created but
// never started, used to be left on its last status with nothing saying why.
func TestUpStartFailureReportsInterruptedServices(t *testing.T) {
	NewScenario(t, "up must say why a service is not started when another service failed to start").
		Step("web fails to start, and api, held by the wait for db, is reported as not started",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			ServiceState("api", "created"),
			OutputContains(`not started: condition "service_healthy" of service "db" not reached: interrupted by the failure of`))
}
