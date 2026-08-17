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

func TestConfigFromEnv(t *testing.T) {
	NewScenario(t, "each config source — file, environment, inline content — must mount with the right content").
		Env("CONFIG=config").
		Step("a file-sourced config mounts its file's content",
			ComposeCmd("run", "from_file"),
			OutputContains("This is my config file")).
		Step("an environment-sourced config mounts the variable's value",
			ComposeCmd("run", "from_env"),
			OutputContains("config")).
		Step("an inline config mounts its interpolated content",
			ComposeCmd("run", "inlined"),
			OutputContains("This is my config")).
		Step("a custom target mounts the config at the requested path",
			ComposeCmd("run", "target"),
			OutputContains("This is my config"))
}
