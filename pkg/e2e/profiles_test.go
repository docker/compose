package e2e

import (
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"strings"
	"testing"
)

func TestExplicitProfileUsage(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-profiles"
	const profileName = "test-profile"

	t.Run("compose up with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: "profiled-service"})
		res.Assert(t, icmd.Expected{Out: "main"})
	})

	t.Run("compose stop with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "stop")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "profiled-service"))
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
	})

	t.Run("compose start with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "start")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: "profiled-service"})
		res.Assert(t, icmd.Expected{Out: "main"})
	})

	t.Run("compose restart with profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "--profile", profileName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: "profiled-service"})
		res.Assert(t, icmd.Expected{Out: "main"})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
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
		res.Assert(t, icmd.Expected{Out: "main"})
		assert.Assert(t, !strings.Contains(res.Combined(), "profiled-service"))
	})

	t.Run("compose stop without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "stop")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "profiled-service"))
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
	})

	t.Run("compose start without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "start")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: "main"})
		assert.Assert(t, !strings.Contains(res.Combined(), "profiled-service"))
	})

	t.Run("compose restart without profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		res.Assert(t, icmd.Expected{Out: "main"})
		assert.Assert(t, !strings.Contains(res.Combined(), "profiled-service"))
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestActiveProfileViaTargetedService(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-profiles-via-target-service"
	const profileName = "test-profile"
	const targetedService = "profiled-service"

	t.Run("compose up with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "up", targetedService, "-d")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
		res.Assert(t, icmd.Expected{Out: targetedService})

		res = c.RunDockerComposeCmd(t, "-p", projectName, "--profile", profileName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
		res.Assert(t, icmd.Expected{Out: targetedService})
	})

	t.Run("compose stop with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "stop", targetedService)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
		assert.Assert(t, !strings.Contains(res.Combined(), targetedService))
	})

	t.Run("compose start with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "start", targetedService)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
		res.Assert(t, icmd.Expected{Out: targetedService})
	})

	t.Run("compose restart with service name", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/profiles/compose.yaml",
			"-p", projectName, "restart")
		res.Assert(t, icmd.Expected{ExitCode: 0})
		res = c.RunDockerComposeCmd(t, "-p", projectName, "ps", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "main"))
		res.Assert(t, icmd.Expected{Out: targetedService})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}
