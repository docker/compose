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
	"fmt"
	"strings"
	"testing"

	testify "github.com/stretchr/testify/assert"
	"gotest.tools/v3/assert"
)

func TestStartStop(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	const projectName = "e2e-start-stop"

	getProjectRegx := func(status string) string {
		// match output with random spaces like:
		// e2e-start-stop      running(3)
		return fmt.Sprintf("%s\\s+%s\\(%d\\)", projectName, status, 2)
	}

	getServiceRegx := func(service string, status string) string {
		// match output with random spaces like:
		// e2e-start-stop-db-1      "echo hello"       db          running
		return fmt.Sprintf("%s-%s-1.+%s\\s+%s", projectName, service, service, status)
	}

	t.Run("Up a project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-simple-1  Started"), res.Combined())

		res = c.RunDockerCmd("compose", "ls", "--all")
		testify.Regexp(t, getProjectRegx("running"), res.Stdout())

		res = c.RunDockerCmd("compose", "--project-name", projectName, "ps")
		testify.Regexp(t, getServiceRegx("simple", "running"), res.Stdout())
		testify.Regexp(t, getServiceRegx("another", "running"), res.Stdout())
	})

	t.Run("stop project", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "stop")

		res := c.RunDockerCmd("compose", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop"), res.Combined())

		res = c.RunDockerCmd("compose", "ls", "--all")
		testify.Regexp(t, getProjectRegx("exited"), res.Stdout())

		res = c.RunDockerCmd("compose", "--project-name", projectName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-words-1"), res.Combined())

		res = c.RunDockerCmd("compose", "--project-name", projectName, "ps", "--all")
		testify.Regexp(t, getServiceRegx("simple", "exited"), res.Stdout())
		testify.Regexp(t, getServiceRegx("another", "exited"), res.Stdout())
	})

	t.Run("start project", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "start")

		res := c.RunDockerCmd("compose", "ls")
		testify.Regexp(t, getProjectRegx("running"), res.Stdout())
	})

	t.Run("pause project", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "pause")

		res := c.RunDockerCmd("compose", "ls", "--all")
		testify.Regexp(t, getProjectRegx("paused"), res.Stdout())
	})

	t.Run("unpause project", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "unpause")

		res := c.RunDockerCmd("compose", "ls")
		testify.Regexp(t, getProjectRegx("running"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
}
