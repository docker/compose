package tests

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestVersionIsSet(t *testing.T) {
	cmd, cleanup, _ := dockerCli.createTestCmd()
	defer cleanup()

	cmd.Command = dockerCli.Command("ecs", "version")
	out := icmd.RunCmd(cmd).Assert(t, icmd.Success).Stdout()
	assert.Check(t, !strings.Contains(out, "unknown"))
}
