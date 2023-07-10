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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/icmd"

	"gotest.tools/v3/assert"
)

func TestWaitOnFaster(t *testing.T) {
	const projectName = "e2e-wait-faster"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/wait/compose.yaml", "--project-name", projectName, "up", "-d")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "wait", "faster")
}

func TestWaitOnSlower(t *testing.T) {
	const projectName = "e2e-wait-slower"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/wait/compose.yaml", "--project-name", projectName, "up", "-d")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "wait", "slower")
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
	const projectName = "e2e-wait-and-drop"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/wait/compose.yaml", "--project-name", projectName, "up", "-d")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "wait", "--down-project", "faster")

	res := c.RunDockerCmd(t, "ps", "--all")
	assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
}
