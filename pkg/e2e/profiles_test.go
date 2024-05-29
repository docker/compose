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

const (
	profiledService = "profiled-service"
	regularService  = "regular-service"
)

func TestExplicitProfileUsage(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-explicit-profiles"
	const profileName = "test-profile"

	t.Run("compose up with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: regularService})
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("compose stop with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "stop")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("compose start with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "start")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: regularService})
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("compose restart with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: regularService})
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestNoProfileUsage(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-no-profiles"

	t.Run("compose up without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: regularService})
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("compose stop without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "stop")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("compose start without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "start")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: regularService})
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("compose restart without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: regularService})
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestActiveProfileViaTargetedService(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-via-target-service-profiles"
	const profileName = "test-profile"

	t.Run("compose up with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "up", profiledService, "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		res.Assert(t, icmd.Expected{Out: profiledService})

		res = c.RunDockerComposeCmd(t, "-p", projectName, "--profile", profileName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("compose stop with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "stop", profiledService)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		assert.Assert(t, !strings.Contains(res.Combined(), profiledService))
	})

	t.Run("compose start with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "start", profiledService)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("compose restart with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), regularService))
		res.Assert(t, icmd.Expected{Out: profiledService})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestDotEnvProfileUsage(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-dotenv-profiles"
	const profileName = "test-profile"

	t.Cleanup(func() {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("compose up with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"--env-file", "./fixtures/profiles/test-profile.env",
			"-p", projectName, "--profile", profileName, "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: regularService})
		res.Assert(t, icmd.Expected{Out: profiledService})
	})
}
