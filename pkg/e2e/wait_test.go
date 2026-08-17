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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestWaitOnFaster(t *testing.T) {
	NewScenario(t, "wait must return once the selected service exits, whatever the others do").
		Step("up starts all services",
			ComposeCmd("up", "-d")).
		Step("wait returns when the fastest service exits",
			ComposeCmd("wait", "faster"),
			ServiceState("faster", "exited"),
			ServiceState("infinity", "running"))
}

func TestWaitOnSlower(t *testing.T) {
	NewScenario(t, "wait must block until the selected service exits, even if others exited first").
		Step("up starts all services",
			ComposeCmd("up", "-d")).
		Step("wait returns when the slower service exits",
			ComposeCmd("wait", "slower"),
			ServiceState("faster", "exited"),
			ServiceState("slower", "exited"),
			ServiceState("infinity", "running"))
}

func TestWaitOnInfinity(t *testing.T) {
	const projectName = "e2e-wait-infinity"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/wait/compose.yaml", "--project-name", projectName, "up", "-d")

	cmd := c.NewDockerComposeCmd(t, "--project-name", projectName, "wait", "infinity")
	r := icmd.StartCmd(cmd)
	assert.NilError(t, r.Error)
	t.Cleanup(func() {
		if r.Cmd.Process != nil {
			_ = r.Cmd.Process.Kill()
		}
	})

	finished := make(chan struct{})
	ticker := time.NewTicker(7 * time.Second)
	go func() {
		_ = r.Cmd.Wait()
		finished <- struct{}{}
	}()

	select {
	case <-finished:
		t.Fatal("wait infinity should not finish")
	case <-ticker.C:
	}
}

func TestWaitAndDrop(t *testing.T) {
	NewScenario(t, "wait --down-project must take the whole project down once the selected service exits").
		Step("up starts all services",
			ComposeCmd("up", "-d")).
		Step("wait --down-project removes every container when the service exits",
			ComposeCmd("wait", "--down-project", "faster"),
			ServiceNotCreated("faster"),
			ServiceNotCreated("slower"),
			ServiceNotCreated("infinity"))
}
