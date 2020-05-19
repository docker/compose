package tests

import (
	"regexp"
	"testing"

	"gotest.tools/assert"
	"gotest.tools/v3/golden"
	"gotest.tools/v3/icmd"
)

func TestInvokePluginFromCLI(t *testing.T) {
	cmd, cleanup, _ := dockerCli.createTestCmd()
	defer cleanup()
	// docker --help should list app as a top command
	cmd.Command = dockerCli.Command("--help")
	icmd.RunCmd(cmd).Assert(t, icmd.Expected{
		Out: "ecs*        Docker ECS (Docker Inc.,",
	})

	// docker app --help prints docker-app help
	cmd.Command = dockerCli.Command("ecs", "--help")
	usage := icmd.RunCmd(cmd).Assert(t, icmd.Success).Combined()

	goldenFile := "plugin-usage.golden"
	golden.Assert(t, usage, goldenFile)

	// docker info should print app version and short description
	cmd.Command = dockerCli.Command("info")
	re := regexp.MustCompile(`ecs: Docker ECS \(Docker Inc\., .*\)`)
	output := icmd.RunCmd(cmd).Assert(t, icmd.Success).Combined()
	assert.Assert(t, re.MatchString(output))
}
