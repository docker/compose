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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

// TestRunBuildOnce tests that services with pull_policy: build are only built once
// when using 'docker compose run', even when they are dependencies.
// This addresses a bug where dependencies were built twice: once in startDependencies
// and once in ensureImagesExists.
func TestRunBuildOnce(t *testing.T) {
	c := NewCLI(t)

	t.Run("dependency with pull_policy build is built only once", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once.yaml", "down", "--rmi", "local", "--remove-orphans")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once.yaml", "run", "--rm", "curl")
		res.Assert(t, icmd.Success)

		// Count how many times nginx was built by looking for its unique RUN command output
		nginxBuilds := strings.Count(res.Combined(), "Building nginx at")

		// nginx should build exactly once, not twice
		assert.Equal(t, nginxBuilds, 1, "nginx dependency should build once, but built %d times", nginxBuilds)
		assert.Assert(t, strings.Contains(res.Combined(), "curl service"))

		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once.yaml", "down", "--remove-orphans")
	})

	t.Run("nested dependencies build only once each", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-nested.yaml", "down", "--rmi", "local", "--remove-orphans")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-nested.yaml", "run", "--rm", "app")
		res.Assert(t, icmd.Success)

		output := res.Combined()

		// Each service should build exactly once
		dbBuilds := strings.Count(output, "DB built at")
		apiBuilds := strings.Count(output, "API built at")
		appBuilds := strings.Count(output, "App built at")

		assert.Equal(t, dbBuilds, 1, "db should build once, built %d times", dbBuilds)
		assert.Equal(t, apiBuilds, 1, "api should build once, built %d times", apiBuilds)
		assert.Equal(t, appBuilds, 1, "app should build once, built %d times", appBuilds)
		assert.Assert(t, strings.Contains(output, "App running"))

		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-nested.yaml", "down", "--remove-orphans")
	})

	t.Run("service with no dependencies builds once", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "down", "--rmi", "local", "--remove-orphans")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "run", "--rm", "simple")
		res.Assert(t, icmd.Success)

		// Should build exactly once
		simpleBuilds := strings.Count(res.Combined(), "Simple service built at")
		assert.Equal(t, simpleBuilds, 1, "simple should build once, built %d times", simpleBuilds)
		assert.Assert(t, strings.Contains(res.Combined(), "Simple service"))

		c.RunDockerComposeCmd(t, "-f", "./fixtures/run-test/build-once-no-deps.yaml", "down", "--remove-orphans")
	})
}
