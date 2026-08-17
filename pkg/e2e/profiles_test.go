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

func TestExplicitProfileUsage(t *testing.T) {
	NewScenario(t, "with --profile, lifecycle commands must act on profiled and regular services alike").
		Step("up with the profile starts both services",
			ComposeCmd("--profile", "test-profile", "up", "-d"),
			ServiceState("regular-service", "running"),
			ServiceState("profiled-service", "running")).
		Step("stop with the profile halts both services",
			ComposeCmd("--profile", "test-profile", "stop"),
			ServiceState("regular-service", "exited"),
			ServiceState("profiled-service", "exited")).
		Step("start with the profile resumes both services",
			ComposeCmd("--profile", "test-profile", "start"),
			ServiceState("regular-service", "running"),
			ServiceState("profiled-service", "running"),
			NotRecreated("regular-service", "profiled-service")).
		Step("restart with the profile restarts both services in place",
			ComposeCmd("--profile", "test-profile", "restart"),
			ServiceState("regular-service", "running"),
			ServiceState("profiled-service", "running"),
			NotRecreated("regular-service", "profiled-service")).
		Step("down removes every container",
			ComposeCmd("--profile", "test-profile", "down"),
			ServiceNotCreated("regular-service"),
			ServiceNotCreated("profiled-service"))
}

func TestNoProfileUsage(t *testing.T) {
	NewScenario(t, "without a profile, lifecycle commands must never materialize the profiled service").
		Step("up starts only the regular service",
			ComposeCmd("up", "-d"),
			ServiceState("regular-service", "running"),
			ServiceNotCreated("profiled-service")).
		Step("stop halts the regular service",
			ComposeCmd("stop"),
			ServiceState("regular-service", "exited"),
			ServiceNotCreated("profiled-service")).
		Step("start resumes the regular service only",
			ComposeCmd("start"),
			ServiceState("regular-service", "running"),
			ServiceNotCreated("profiled-service"),
			NotRecreated("regular-service")).
		Step("restart touches the regular service only",
			ComposeCmd("restart"),
			ServiceState("regular-service", "running"),
			ServiceNotCreated("profiled-service"),
			NotRecreated("regular-service")).
		Step("down removes every container",
			ComposeCmd("down"),
			ServiceNotCreated("regular-service"))
}

func TestActiveProfileViaTargetedService(t *testing.T) {
	NewScenario(t, "targeting a profiled service must activate its profile implicitly, and only for it").
		Step("up on the profiled service starts it without the regular one",
			ComposeCmd("up", "-d", "profiled-service"),
			ServiceState("profiled-service", "running"),
			ServiceNotCreated("regular-service")).
		Step("stop on the profiled service halts it",
			ComposeCmd("stop", "profiled-service"),
			ServiceState("profiled-service", "exited"),
			ServiceNotCreated("regular-service")).
		Step("start on the profiled service resumes it",
			ComposeCmd("start", "profiled-service"),
			ServiceState("profiled-service", "running"),
			ServiceNotCreated("regular-service"),
			NotRecreated("profiled-service")).
		Step("restart keeps acting on the existing containers only",
			ComposeCmd("restart"),
			ServiceState("profiled-service", "running"),
			ServiceNotCreated("regular-service"),
			NotRecreated("profiled-service"))
}

func TestDotEnvProfileUsage(t *testing.T) {
	NewScenario(t, "COMPOSE_PROFILES from the project's .env must activate the profile").
		Step("up starts both services, the profile coming from .env",
			ComposeCmd("up", "-d"),
			ServiceState("regular-service", "running"),
			ServiceState("profiled-service", "running"))
}
