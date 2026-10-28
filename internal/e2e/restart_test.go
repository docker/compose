//go:build e2e

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

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func assertServiceStatus(t *testing.T, projectName, service, status string, ps string) {
	// match output with random spaces like:
	// e2e-start-stop-db-1      alpine:latest "echo hello"     db	1 minutes ago	Exited (0) 1 minutes ago
	regx := fmt.Sprintf("%s-%s-1.+%s\\s+.+%s.+", projectName, service, service, status)
	assert.Assert(t, is.Regexp(regx, ps))
}

func TestRestart(t *testing.T) {
	// the service's first run creates a lock file and exits at once; any
	// later run of the same container finds the lock and sleeps forever, so
	// staying up after `restart` proves the same container was restarted
	NewScenario(t, "restart must bring an exited service back up, restarting the same container").
		Step("up starts the service, whose first run exits at once",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "exited"), 10*time.Second)).
		Step("restart brings the service back up, reusing the container",
			ComposeCmd("restart"),
			Eventually(ServiceState("app", "running"), 10*time.Second),
			NotRecreated("app"))
}

func TestRestartWithDependencies(t *testing.T) {
	c := NewCLI(t, WithEnv(
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
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Restarting", baseService)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Healthy", baseService)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Started", depWithRestart)), out)
	assert.Assert(t, !strings.Contains(out, depNoRestart), out)

	c = NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-restart-deps",
		"LABEL=recreate",
	))
	res = c.RunDockerComposeCmd(t, "-f", "./fixtures/restart-test/compose-depends-on.yaml", "up", "-d")
	out = res.Combined()
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Stopped", depWithRestart)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Recreated", baseService)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Healthy", baseService)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Started", depWithRestart)), out)
	assert.Assert(t, strings.Contains(out, fmt.Sprintf("Container e2e-restart-deps-%s-1 Running", depNoRestart)), out)
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
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-restart-profiles-test-1 Started"), res.Combined())
}
