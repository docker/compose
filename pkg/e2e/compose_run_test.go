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
	defer c.cleanupWithDown(t, "run-test")

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
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "down", "--remove-orphans", "--rmi", "all")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "run", "--quiet-pull", "backend")
		assert.Assert(t, !strings.Contains(res.Combined(), "Pull complete"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Pulled"), res.Combined())
	})

	t.Run("COMPOSE_PROGRESS quiet", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "down", "--remove-orphans", "--rmi", "all")
		res.Assert(t, icmd.Success)

		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/run-test/quiet-pull.yaml", "run", "backend")
		res = icmd.RunCmd(cmd, func(c *icmd.Cmd) {
			c.Env = append(c.Env, "COMPOSE_PROGRESS=quiet")
		})
		assert.Assert(t, !strings.Contains(res.Combined(), "Pull complete"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "Pulled"), res.Combined())
	})

	t.Run("--pull", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/pull.yaml", "down", "--remove-orphans", "--rmi", "all")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/pull.yaml", "run", "--pull", "always", "backend")
		assert.Assert(t, strings.Contains(res.Combined(), "backend Pulling"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "backend Pulled"), res.Combined())
	})

	t.Run("compose run --env-from-file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "--env-from-file", "./fixtures/run-test/run.env",
			"front", "env")
		res.Assert(t, icmd.Expected{Out: "FOO=BAR"})
	})

	t.Run("compose run -rm with stop signal", func(t *testing.T) {
		projectName := "run-test"
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "-f", "./fixtures/ps-test/compose.yaml", "run", "--rm", "-d", "nginx")
		res.Assert(t, icmd.Success)

		res = c.RunDockerCmd(t, "ps", "--quiet", "--filter", "name=run-test-nginx")
		containerID := strings.TrimSpace(res.Stdout())

		res = c.RunDockerCmd(t, "stop", containerID)
		res.Assert(t, icmd.Success)
		res = c.RunDockerCmd(t, "ps", "--all", "--filter", "name=run-test-nginx", "--format", "'{{.Names}}'")
		assert.Assert(t, !strings.Contains(res.Stdout(), "run-test-nginx"), res.Stdout())
	})

	t.Run("compose run --env", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "--env", "FOO=BAR",
			"front", "env")
		res.Assert(t, icmd.Expected{Out: "FOO=BAR"})
	})

	t.Run("compose run --build", func(t *testing.T) {
		c.cleanupWithDown(t, "run-test", "--rmi=local")
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/compose.yaml", "run", "build", "echo", "hello world")
		res.Assert(t, icmd.Expected{Out: "hello world"})
	})

	t.Run("compose run with piped input detection", func(t *testing.T) {
		if composeStandaloneMode {
			t.Skip("Skipping test compose with piped input detection in standalone mode")
		}
		// Test that piped input is properly detected and TTY is automatically disabled
		// This tests the logic added in run.go that checks dockerCli.In().IsTerminal()
		cmd := c.NewCmd("sh", "-c", "echo 'piped-content' | docker compose -f ./fixtures/run-test/piped-test.yaml run --rm piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{Out: "piped-content"})
		res.Assert(t, icmd.Success)
	})

	t.Run("compose run piped input should not allocate TTY", func(t *testing.T) {
		if composeStandaloneMode {
			t.Skip("Skipping test compose with piped input detection in standalone mode")
		}
		// Test that when stdin is piped, the container correctly detects no TTY
		// This verifies that the automatic noTty=true setting works correctly
		cmd := c.NewCmd("sh", "-c", "echo '' | docker compose -f ./fixtures/run-test/piped-test.yaml run --rm tty-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{Out: "No TTY detected"})
		res.Assert(t, icmd.Success)
	})

	t.Run("compose run piped input with explicit --tty should fail", func(t *testing.T) {
		if composeStandaloneMode {
			t.Skip("Skipping test compose with piped input detection in standalone mode")
		}
		// Test that explicitly requesting TTY with piped input fails with proper error message
		// This should trigger the "input device is not a TTY" error
		cmd := c.NewCmd("sh", "-c", "echo 'test' | docker compose -f ./fixtures/run-test/piped-test.yaml run --rm --tty piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "the input device is not a TTY",
		})
	})

	t.Run("compose run piped input with --no-TTY=false should fail", func(t *testing.T) {
		if composeStandaloneMode {
			t.Skip("Skipping test compose with piped input detection in standalone mode")
		}
		// Test that explicitly disabling --no-TTY (i.e., requesting TTY) with piped input fails
		// This should also trigger the "input device is not a TTY" error
		cmd := c.NewCmd("sh", "-c", "echo 'test' | docker compose -f ./fixtures/run-test/piped-test.yaml run --rm --no-TTY=false piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "the input device is not a TTY",
		})
	})
}
