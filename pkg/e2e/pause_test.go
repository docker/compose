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

func TestPause(t *testing.T) {
	NewScenario(t, "pause must freeze only the targeted service, unpause must resume it in place").
		Compose(`
services:
  a:
    image: alpine
    init: true
    command: sleep infinity
  b:
    image: alpine
    init: true
    command: sleep infinity
`).
		Step("up starts both services",
			ComposeCmd("up", "-d"),
			ServiceState("a", "running"),
			ServiceState("b", "running")).
		Step("pause freezes the targeted service, the other keeps running",
			ComposeCmd("pause", "a"),
			ServiceState("a", "paused"),
			ServiceState("b", "running")).
		Step("unpause resumes the paused service without recreating anything",
			ComposeCmd("unpause", "a"),
			ServiceState("a", "running"),
			ServiceState("b", "running"),
			NotRecreated("a", "b"))
}

func TestPauseServiceNotRunning(t *testing.T) {
	// TODO: `docker pause` errors in this case, should Compose be consistent?
	NewScenario(t, "pause of a service with no container must succeed as a no-op").
		Compose(`
services:
  a:
    image: alpine
    init: true
    command: sleep infinity
`).
		Step("pause without any container is accepted",
			ComposeCmd("pause", "a"))
}

func TestPauseServiceAlreadyPaused(t *testing.T) {
	NewScenario(t, "pausing an already-paused service must fail").
		Compose(`
services:
  a:
    image: alpine
    init: true
    command: sleep infinity
`).
		Step("up starts the service",
			ComposeCmd("up", "-d"),
			ServiceState("a", "running")).
		Step("a first pause freezes the service",
			ComposeCmd("pause", "a"),
			ServiceState("a", "paused")).
		Step("a second pause is rejected",
			ComposeCmd("pause", "a").MayFail(),
			ExitCode(1),
			OutputContains("already paused"))
}

func TestPauseServiceDoesNotExist(t *testing.T) {
	NewScenario(t, "pause of a service unknown to the model must be rejected").
		Compose(`
services:
  a:
    image: alpine
    init: true
    command: sleep infinity
`).
		Step("pause of an unknown service is rejected",
			ComposeCmd("pause", "does_not_exist").MayFail(),
			ExitCode(1),
			OutputContains("no such service: does_not_exist"))
}
