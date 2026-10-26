//go:build e2e

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

// Regression test for https://github.com/docker/compose/issues/14386: a
// container created beforehand and only started by `compose start` got
// DOCKER_CONFIG pointing at /run/secrets/docker, without the config file the
// use_api_socket option is meant to provide there. The file is injected when
// the container starts, from the project given to the start, which did not
// carry the option.
func TestStartCreatedUseAPISocketService(t *testing.T) {
	NewScenario(t, "start must provide the use_api_socket config file to a container created beforehand").
		Step("create makes the container without starting it",
			ComposeCmd("create"),
			ServiceState("app", "created")).
		Step("start runs it with the config file in place",
			ComposeCmd("start"),
			Eventually(ServiceState("app", "running"), 10*time.Second),
			ExecOutputContains("app", "[ -f /run/secrets/docker/config.json ] && echo apisocket-config-present", "apisocket-config-present"))
}
