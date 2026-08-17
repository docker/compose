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
		Files(`
-- compose.yaml --
services:
  from_env:
    image: alpine
    configs:
      - source: from_env
    command: cat /from_env

  from_file:
    image: alpine
    configs:
      - source: from_file
    command: cat /from_file

  inlined:
    image: alpine
    configs:
      - source: inlined
    command: cat /inlined

  target:
    image: alpine
    configs:
      - source: inlined
        target: /target
    command: cat /target

configs:
  from_env:
    environment: CONFIG
  from_file:
    file: config.txt
  inlined:
    content: This is my $CONFIG
-- config.txt --
This is my config file
`).
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
