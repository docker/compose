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

// TestUpUnhealthyDependencyNamesTheConditionNotMet locks what a service held by
// a wait reads when the dependency it waits for fails: the condition that was
// not met, not an interruption nor the internal name of the wait.
func TestUpUnhealthyDependencyNamesTheConditionNotMet(t *testing.T) {
	NewScenario(t, "up must name the depends_on condition a dependency did not meet").
		Step("db turns unhealthy, and api, held by the wait for it, is reported as not started",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			ServiceState("api", "created"),
			OutputContains(`not started: condition "service_healthy" of service "db" not met`),
			OutputNotContains("wait:"))
}

// TestUpOptionalDependentIsNotBlamedOnTheWaitItDeclaredOptional locks
// docker/compose#14507: the wait on a dependency is shared by every dependent
// awaiting the same condition, and one requiring it makes it strict for all.
// When it fails, the dependent that declared the dependency optional is given
// up with the others (up fails fast), but it is not reported as held by a
// condition that does not apply to it: it is interrupted, and a warning says
// why.
func TestUpOptionalDependentIsNotBlamedOnTheWaitItDeclaredOptional(t *testing.T) {
	NewScenario(t, "up must not report an optional dependent as held by the unmet condition of a dependency it declared optional").
		Step("db turns unhealthy: needs_db is blocked by it, optional_db only interrupted",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			OutputContains(`needs_db-1 not started: condition "service_healthy" of service "db" not met`),
			OutputContains(`optional_db-1 not started: interrupted by the unmet condition "service_healthy" of service "db"`),
			OutputNotContains(`optional_db-1 not started: condition "service_healthy" of service "db" not met`),
			OutputContains(`level=warning msg="optional dependency \"db\" did not meet condition \"service_healthy\"`))
}
