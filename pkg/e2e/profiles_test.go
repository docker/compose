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
