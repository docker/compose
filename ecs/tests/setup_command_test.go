package tests

import (
	"strings"
	"testing"

	"gotest.tools/assert"
	"gotest.tools/v3/golden"
	"gotest.tools/v3/icmd"
)

func TestSetupMandatoryArguments(t *testing.T) {
	cmd, cleanup := dockerCli.createTestCmd()
	defer cleanup()

	cmd.Command = dockerCli.Command("ecs", "setup")
	icmd.RunCmd(cmd).Assert(t, icmd.Expected{
		ExitCode: 1,
		Err:      "required flag(s) \"cluster\", \"profile\", \"region\" not set",
	})
}
func TestDefaultAwsContextName(t *testing.T) {
	cmd, cleanup := dockerCli.createTestCmd()
	defer cleanup()

	cmd.Command = dockerCli.Command("ecs", "setup", "--cluster", "clusterName", "--profile", "profileName",
		"--region", "regionName")
	icmd.RunCmd(cmd).Assert(t, icmd.Success)

	cmd.Command = dockerCli.Command("context", "inspect", "aws")
	output := icmd.RunCmd(cmd).Assert(t, icmd.Success).Combined()
	expected := golden.Get(t, "context-inspect.golden")
	assert.Assert(t, strings.HasPrefix(output, string(expected)))
}
