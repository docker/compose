package e2e

import (
	"testing"

	"gotest.tools/v3/icmd"
)

func TestIncludeEnvFileInterpolation(t *testing.T) {
	c := NewParallelCLI(t)
	defer c.cleanupWithDown(t, "include-envfile-interpolation")

	// this must succeed: top-level env_file value VAR=1 should be applied
	// when evaluating subproject's env_file which contains MYVAR=${VAR?}
	c.RunDockerComposeCmd(t, "-f", "./fixtures/include-envfile-interpolation/compose.yml", "config")

	// additionally run `run` to ensure interpolation for the env file is available when creating containers
	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/include-envfile-interpolation/compose.yml", "run", "--rm", "app", "sh", "-c", "echo $MYVAR")
	res.Assert(t, icmd.Expected{Out: "1"})
}
