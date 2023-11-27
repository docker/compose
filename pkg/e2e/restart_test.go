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

func assertServiceStatus(t *testing.T, projectName, service, status string, ps string) {
	// match output with random spaces like:
	// e2e-start-stop-db-1      alpine:latest "echo hello"     db	1 minutes ago	Exited (0) 1 minutes ago
	regx := fmt.Sprintf("%s-%s-1.+%s\\s+.+%s.+", projectName, service, service, status)
	testify.Regexp(t, regx, ps)
}

func TestRestart(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-restart"

	t.Run("Up a project", func(t *testing.T) {
		// This is just to ensure the containers do NOT exist
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose.yaml", "--project-name", projectName, "up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-restart-restart-1  Started"), res.Combined())

		c.WaitForCmdResult(t, c.NewDockerComposeCmd(t, "--project-name", projectName, "ps", "-a", "--format",
			"json"),
			StdoutContains(`"State":"exited"`), 10*time.Second, 1*time.Second)

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a")
		assertServiceStatus(t, projectName, "restart", "Exited", res.Stdout())

		c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose.yaml", "--project-name", projectName, "restart")

		// Give the same time but it must NOT exit
		time.Sleep(time.Second)

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps")
		assertServiceStatus(t, projectName, "restart", "Up", res.Stdout())

		// Clean up
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestRestartWithDependencies(t *testing.T) {
	c := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-restart-deps",
	))
	baseService := "nginx"
	depWithRestart := "with-restart"
	depNoRestart := "no-restart"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "down", "--remove-orphans")
	})

	c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose-depends-on.yaml", "up", "-d")

	res := c.RunDockerComposeCmd(t, "restart", baseService)
	out := res.Combined()
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1  Started", baseService)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1  Started", depWithRestart)), out)
	assert.Assert(t, !strings.Contains(out, depNoRestart), out)
}

func TestRestartWithProfiles(t *testing.T) {
	c := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-restart-profiles",
	))

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "down", "--remove-orphans")
	})

	c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose.yaml", "--profile", "test", "up", "-d")

	res := c.RunDockerComposeCmd(t, "restart", "test")
	fmt.Println(res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-restart-profiles-test-1  Started"), res.Combined())
}
