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
	"time"

	testify "github.com/stretchr/testify/assert"
	"gotest.tools/v3/assert"
)

func TestRestart(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-restart"

	getServiceRegx := func(service string, status string) string {
		// match output with random spaces like:
		// e2e-start-stop-db-1      "echo hello"     db      running
		return fmt.Sprintf("%s-%s-1.+%s\\s+%s", projectName, service, service, status)
	}

	t.Run("Up a project", func(t *testing.T) {
		// This is just to ensure the containers do NOT exist
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose.yaml", "--project-name", projectName, "up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-restart-restart-1  Started"), res.Combined())

		c.WaitForCmdResult(t, c.NewDockerComposeCmd(t, "--project-name", projectName, "ps", "-a", "--format",
			"json"),
			StdoutContains(`"State":"exited"`), 10*time.Second, 1*time.Second)

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a")
		testify.Regexp(t, getServiceRegx("restart", "exited"), res.Stdout())

		c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose.yaml", "--project-name", projectName, "restart")

		// Give the same time but it must NOT exit
		time.Sleep(time.Second)

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps")
		testify.Regexp(t, getServiceRegx("restart", "running"), res.Stdout())

		// Clean up
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}
