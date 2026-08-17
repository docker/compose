/*
   Copyright 2022 Docker Compose CLI authors

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

func TestRecreateWithNoDeps(t *testing.T) {
	NewScenario(t, "up --force-recreate --no-deps must replace the service without touching its healthy dependency").
		Step("up starts the service once its dependency is healthy",
			ComposeCmd("up", "-d", "--wait").Within(60*time.Second),
			ServiceState("my-service", "running"),
			ServiceHealthy("dep")).
		Step("force-recreate with --no-deps replaces only the service",
			ComposeCmd("up", "-d", "--force-recreate", "--no-deps", "my-service"),
			ServiceState("my-service", "running"),
			Recreated("my-service"),
			NotRecreated("dep"))
}
