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

const execCompose = `
services:
  simple:
    image: alpine
    init: true
    command: top
  another:
    image: alpine
    init: true
    command: top
`

func TestLocalComposeExec(t *testing.T) {
	NewScenario(t, "exec must run in the service container, propagate the exit code and pass only requested env").
		Compose(execCompose).
		Step("up starts the service",
			ComposeCmd("up", "-d"),
			ServiceState("simple", "running")).
		Step("a successful command exits 0",
			ComposeCmd("exec", "simple", "/bin/true")).
		Step("a failing command's exit code is propagated",
			ComposeCmd("exec", "simple", "/bin/false").MayFail(),
			ExitCode(1)).
		Step("exec -e forwards the variable when set on the caller",
			ComposeCmd("exec", "-e", "FOO", "simple", "/usr/bin/env").WithEnv("FOO=BAR"),
			OutputContains("FOO=BAR")).
		Step("exec -e without a value does not leak an empty variable",
			ComposeCmd("exec", "-e", "FOO", "simple", "/usr/bin/env"),
			OutputNotContains("FOO="))
}

func TestLocalComposeExecOneOff(t *testing.T) {
	NewScenario(t, "exec must reach a one-off container, but --index must only match numbered replicas").
		Compose(execCompose).
		Step("run starts a detached one-off",
			ComposeCmd("run", "-d", "simple", "top"),
			OneOffState("simple", "running")).
		Step("exec lands in the one-off container",
			ComposeCmd("exec", "-e", "FOO", "simple", "/usr/bin/env"),
			OutputNotContains("FOO=")).
		Step("exec --index rejects a one-off: it is not replica #1",
			ComposeCmd("exec", "--index", "1", "-e", "FOO", "simple", "/usr/bin/env").MayFail(),
			ExitCode(1),
			OutputContains(`service "simple" is not running container #1`))
}
