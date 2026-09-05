//go:build e2e && !windows

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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestCascadeStop(t *testing.T) {
	NewScenario(t, "up --abort-on-container-exit must stop the project on the first exit, and exit 0 without --exit-code-from").
		Step("up aborts once a container exits, reporting which one",
			ComposeCmd("up", "--abort-on-container-exit").Within(60*time.Second),
			OutputContains("exit-1 exited with code 0"),
			ServiceState("running", "exited"))
}

func TestCascadeFail(t *testing.T) {
	NewScenario(t, "up --abort-on-container-failure must propagate the failing container's exit code").
		Step("up keeps going on clean exits and aborts on the failure, with its exit code",
			ComposeCmd("up", "--abort-on-container-failure").MayFail().Within(60*time.Second),
			ExitCode(111),
			OutputContains("exit-1 exited with code 0"),
			OutputContains("fail-1 exited with code 111"),
			ServiceState("running", "exited"))
}

func TestCascadeIgnoresOneOffContainer(t *testing.T) {
	const projectName = "compose-e2e-cascade-oneoff"
	c := NewCLI(t, WithEnv("COMPOSE_PROJECT_NAME="+projectName))
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "down")
	})

	cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/cascade/compose.yaml",
		"up", "--abort-on-container-exit", "--menu=false", "running")
	res := icmd.StartCmd(cmd)
	t.Cleanup(func() {
		_ = res.Cmd.Process.Kill()
	})

	poll.WaitOn(t, expectOutput(res, "Attaching to running-1"),
		poll.WithDelay(500*time.Millisecond), poll.WithTimeout(30*time.Second))

	c.RunDockerComposeCmd(t, "-f", "./fixtures/cascade/compose.yaml",
		"run", "--rm", "--no-deps", "running", "/bin/true")

	time.Sleep(3 * time.Second)

	assert.Assert(t, !strings.Contains(res.Combined(), "Aborting on container exit"), res.Combined())
	RequireServiceState(t, c, "running", "running")
}
