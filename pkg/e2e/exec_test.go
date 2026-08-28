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
)

func TestExec(t *testing.T) {
	NewScenario(t, "exec must prefer the service container over a one-off, falling back to the one-off when alone").
		Step("run starts a detached one-off with its own command",
			ComposeCmd("run", "-d", "test", "cat"),
			OneOffState("test", "running")).
		Step("exec --index=1 finds no numbered replica",
			ComposeCmd("exec", "--index=1", "test", "ps").MayFail(),
			ExitCode(1),
			OutputContains(`service "test" is not running container #1`)).
		Step("with only a one-off around, exec lands in it",
			ComposeCmd("exec", "test", "ps"),
			StdoutContains("cat")).
		Step("up starts the service container",
			ComposeCmd("up", "-d"),
			ServiceState("test", "running")).
		Step("exec now selects the service container",
			ComposeCmd("exec", "test", "ps"),
			StdoutContains("tail"))
}
