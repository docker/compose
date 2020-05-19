package tests

import (
	"testing"

	"gotest.tools/v3/icmd"
)

func TestExitErrorCode(t *testing.T) {
	cmd, cleanup, _ := dockerCli.createTestCmd()
	defer cleanup()

	cmd.Command = dockerCli.Command("ecs", "unknown_command")
	icmd.RunCmd(cmd).Assert(t, icmd.Expected{
		ExitCode: 1,
		Err:      "\"unknown_command\" is not a docker ecs command\nSee 'docker ecs --help'",
	})
}
