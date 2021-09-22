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
	c := NewParallelE2eCLI(t, binDir)

	t.Run("compose run", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "run", "back")
		lines := Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello there!!", res.Stdout())
		assert.Assert(t, !strings.Contains(res.Combined(), "orphan"))
		res = c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "run", "back", "echo", "Hello one more time")
		lines = Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello one more time", res.Stdout())
		assert.Assert(t, !strings.Contains(res.Combined(), "orphan"))
	})

	t.Run("check run container exited", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--all")
		lines := Lines(res.Stdout())
		var runContainerID string
		var truncatedSlug string
		for _, line := range lines {
			fields := strings.Fields(line)
			containerID := fields[len(fields)-1]
			assert.Assert(t, !strings.HasPrefix(containerID, "run-test_front"))
			if strings.HasPrefix(containerID, "run-test_back") {
				// only the one-off container for back service
				assert.Assert(t, strings.HasPrefix(containerID, "run-test_back_run_"), containerID)
				truncatedSlug = strings.Replace(containerID, "run-test_back_run_", "", 1)
				runContainerID = containerID
			}
			if strings.HasPrefix(containerID, "run-test-db-1") {
				assert.Assert(t, strings.Contains(line, "Up"), line)
			}
		}
		assert.Assert(t, runContainerID != "")
		res = c.RunDockerCmd("inspect", runContainerID)
		res.Assert(t, icmd.Expected{Out: ` "Status": "exited"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "run-test"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "True",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.slug": "` + truncatedSlug})
	})

	t.Run("compose run --rm", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "run", "--rm", "back", "echo", "Hello again")
		lines := Lines(res.Stdout())
		assert.Equal(t, lines[len(lines)-1], "Hello again", res.Stdout())

		res = c.RunDockerCmd("ps", "--all")
		assert.Assert(t, strings.Contains(res.Stdout(), "run-test_back"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "down")
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Stdout(), "run-test"), res.Stdout())
	})

	t.Run("compose run --volumes", func(t *testing.T) {
		wd, err := os.Getwd()
		assert.NilError(t, err)
		res := c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "run", "--volumes", wd+":/foo", "back", "/bin/sh", "-c", "ls /foo")
		res.Assert(t, icmd.Expected{Out: "compose_run_test.go"})

		res = c.RunDockerCmd("ps", "--all")
		assert.Assert(t, strings.Contains(res.Stdout(), "run-test_back"), res.Stdout())
	})

	t.Run("compose run --publish", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "run", "--publish", "8081:80", "-d", "back", "/bin/sh", "-c", "sleep 1")
		res := c.RunDockerCmd("ps")
		assert.Assert(t, strings.Contains(res.Stdout(), "8081->80/tcp"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/run-test/compose.yaml", "down")
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Stdout(), "run-test"), res.Stdout())
	})
}
