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
	"testing"
	"time"
)

func TestUmask(t *testing.T) {
	NewScenario(t, "umask must set the file mode creation mask of the entrypoint and of exec processes").
		Requires(EngineAPIVersionAtLeast("1.56")).
		Step("up starts the service with its umask, which the entrypoint and an exec process both see",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "running"), 10*time.Second),
			Eventually(ExecOutputContains("app", "cat /tmp/entrypoint-umask", "0027"), 10*time.Second),
			ExecOutputContains("app", "umask", "0027"))
}
