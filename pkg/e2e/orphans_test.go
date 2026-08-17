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

func TestRemoveOrphans(t *testing.T) {
	// Without COMPOSE_REMOVE_ORPHANS, up only warns about the leftover
	// one-off; the .env setting must be picked up from the project directory
	// and turn the warning into a removal.
	NewScenario(t, "up must honor COMPOSE_REMOVE_ORPHANS declared in the project's .env").
		Step("run leaves an exited one-off container behind",
			ComposeCmd("run", "orphan"),
			OneOffState("orphan", "exited")).
		Step("up removes the leftover one-off",
			ComposeCmd("up", "-d"),
			ServiceState("test", "running"),
			OneOffsRemoved("orphan"))
}
