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
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeRun(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("compose run", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "back")
		lines := Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello there!!", res.Stdout())
		assert.Assert(t, !strings.Contains(res.Combined(), "orphan"))
		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "back", "echo",
			"Hello one more time")
		lines = Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello one more time", res.Stdout())
		assert.Assert(t, strings.Contains(res.Combined(), "orphan"))
	})

	t.Run("check run container exited", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		lines := Lines(res.Stdout())
		var runContainerID string
		var truncatedSlug string
		for _, line := range lines {
			fields := strings.Fields(line)
			containerID := fields[len(fields)-1]
			assert.Assert(t, !strings.HasPrefix(containerID, "run-test-front"))
			if strings.HasPrefix(containerID, "run-test-back") {
				// only the one-off container for back service
				assert.Assert(t, strings.HasPrefix(containerID, "run-test-back-run-"), containerID)
				truncatedSlug = strings.Replace(containerID, "run-test-back-run-", "", 1)
				runContainerID = containerID
			}
			if strings.HasPrefix(containerID, "run-test-db-1") {
				assert.Assert(t, strings.Contains(line, "Up"), line)
			}
		}
		assert.Assert(t, runContainerID != "")
		res = c.RunDockerCmd(t, "inspect", runContainerID)
		res.Assert(t, icmd.Expected{Out: ` "Status": "exited"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "run-test"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "True",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.slug": "` + truncatedSlug})
	})

	t.Run("compose run --rm", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "--rm", "back", "echo",
			"Hello again")
		lines := Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello again", res.Stdout())

		res = c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, strings.Contains(res.Stdout(), "run-test-back"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "down", "--remove-orphans")
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Stdout(), "run-test"), res.Stdout())
	})

	t.Run("compose run --volumes", func(t *testing.T) {
		wd, err := os.Getwd()
		assert.NilError(t, err)
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "--volumes", wd+":/foo",
			"back", "/bin/sh", "-c", "ls /foo")
		res.Assert(t, icmd.Expected{Out: "compose_run_test.go"})

		res = c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, strings.Contains(res.Stdout(), "run-test-back"), res.Stdout())
	})

	t.Run("compose run --publish", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/ports.yaml", "run", "--publish", "8081:80", "-d", "back",
			"/bin/sh", "-c", "sleep 1")
		res := c.RunDockerCmd(t, "ps")
		assert.Assert(t, strings.Contains(res.Stdout(), "8081->80/tcp"), res.Stdout())
		assert.Assert(t, !strings.Contains(res.Stdout(), "8082->80/tcp"), res.Stdout())
	})

	t.Run("compose run --service-ports", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/ports.yaml", "run", "--service-ports", "-d", "back",
			"/bin/sh", "-c", "sleep 1")
		res := c.RunDockerCmd(t, "ps")
		assert.Assert(t, strings.Contains(res.Stdout(), "8082->80/tcp"), res.Stdout())
	})

	t.Run("compose run orphan", func(t *testing.T) {
		// Use different compose files to get an orphan container
		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/orphan.yaml", "run", "simple")
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "back", "echo", "Hello")
		assert.Assert(t, strings.Contains(res.Combined(), "orphan"))

		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "back", "echo", "Hello")
		res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "COMPOSE_IGNORE_ORPHANS=True")
		})
		assert.Assert(t, !strings.Contains(res.Combined(), "orphan"))
	})

	t.Run("down", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "down")
		icmd.RunCmd(cmd, func(c *icmd.Cmd) {
			c.Env = append(c.Env, "COMPOSE_REMOVE_ORPHANS=True")
		})
		res := c.RunDockerCmd(t, "ps", "--all")

		assert.Assert(t, !strings.Contains(res.Stdout(), "run-test"), res.Stdout())
	})

	t.Run("run starts only container and dependencies", func(t *testing.T) {
		// ensure that even if another service is up run does not start it: https://github.com/docker/compose/issues/9459
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/deps.yaml", "up", "service_b", "--menu=false")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/deps.yaml", "run", "service_a")
		assert.Assert(t, strings.Contains(res.Combined(), "shared_dep"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "service_b"), res.Combined())

		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/deps.yaml", "down", "--remove-orphans")
	})

	t.Run("run without dependencies", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/deps.yaml", "run", "--no-deps", "service_a")
		assert.Assert(t, !strings.Contains(res.Combined(), "shared_dep"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "service_b"), res.Combined())

		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/deps.yaml", "down", "--remove-orphans")
	})

	t.Run("run with not required dependency", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/deps-not-required.yaml", "run", "foo")
		assert.Assert(t, strings.Contains(res.Combined(), "foo"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "bar"), res.Combined())

		c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/deps-not-required.yaml", "down", "--remove-orphans")
	})

	t.Run("--quiet-pull", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "down", "--rmi", "all")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "run", "--quiet-pull", "backend")
		assert.Assert(t, !strings.Contains(res.Combined(), "Pull complete"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Pulled"), res.Combined())
	})
}
