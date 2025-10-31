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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestRunBuildOnce tests that services with pull_policy: build are only built once
// when using 'docker compose run', even when they are dependencies.
// This addresses a bug where dependencies were built twice: once in startDependencies
// and once in ensureImagesExists.
func TestRunBuildOnce(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("dependency with pull_policy build is built only once", func(t *testing.T) {
		projectName := randomProjectName("build-once")
		_ = c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once.yaml", "down", "--rmi", "local", "--remove-orphans", "-v")
		res := c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once.yaml", "--verbose", "run", "--build", "--rm", "curl")

		// Count how many times nginx was built by looking for its unique RUN command output
		nginxBuilds := strings.Count(res.Stdout(), "Building nginx at")

		// nginx should build exactly once, not twice
		assert.Equal(t, nginxBuilds, 1, "nginx dependency should build once, but built %d times", nginxBuilds)
		assert.Assert(t, strings.Contains(res.Stdout(), "curl service"))

		c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once.yaml", "down", "--remove-orphans")
	})

	t.Run("nested dependencies build only once each", func(t *testing.T) {
		projectName := randomProjectName("build-nested")
		_ = c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-nested.yaml", "down", "--rmi", "local", "--remove-orphans", "-v")
		res := c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-nested.yaml", "--verbose", "run", "--build", "--rm", "app")

		output := res.Stdout()

		dbBuildMarker := fmt.Sprintf("naming to docker.io/library/%s-db", projectName)
		apiBuildMarker := fmt.Sprintf("naming to docker.io/library/%s-api", projectName)
		appBuildMarker := fmt.Sprintf("naming to docker.io/library/%s-app", projectName)

		dbBuilds := strings.Count(output, dbBuildMarker)
		apiBuilds := strings.Count(output, apiBuildMarker)
		appBuilds := strings.Count(output, appBuildMarker)

		assert.Equal(t, dbBuilds, 1, "db should build once, built %d times\nOutput:\n%s", dbBuilds, output)
		assert.Equal(t, apiBuilds, 1, "api should build once, built %d times\nOutput:\n%s", apiBuilds, output)
		assert.Equal(t, appBuilds, 1, "app should build once, built %d times\nOutput:\n%s", appBuilds, output)
		assert.Assert(t, strings.Contains(output, "App running"))

		c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-nested.yaml", "down", "--rmi", "local", "--remove-orphans", "-v")
	})

	t.Run("service with no dependencies builds once", func(t *testing.T) {
		projectName := randomProjectName("build-simple")
		_ = c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "down", "--rmi", "local", "--remove-orphans")
		res := c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "run", "--build", "--rm", "simple")

		// Should build exactly once
		simpleBuilds := strings.Count(res.Stdout(), "Simple service built at")
		assert.Equal(t, simpleBuilds, 1, "simple should build once, built %d times", simpleBuilds)
		assert.Assert(t, strings.Contains(res.Stdout(), "Simple service"))

		c.RunDockerComposeCmd(t, "-p", projectName, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "down", "--remove-orphans")
	})
}

// randomProjectName generates a unique project name for parallel test execution
// Format: prefix-<8 random hex chars> (e.g., "build-once-3f4a9b2c")
func randomProjectName(prefix string) string {
	b := make([]byte, 4) // 4 bytes = 8 hex chars
	rand.Read(b)         //nolint:errcheck
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}
