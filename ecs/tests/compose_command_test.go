package tests

import (
	"testing"

	"gotest.tools/v3/fs"
	"gotest.tools/v3/golden"
	"gotest.tools/v3/icmd"
)

const (
	composeFileName = "compose.yaml"
)

func TestSimpleConvert(t *testing.T) {
	cmd, cleanup := dockerCli.createTestCmd()
	defer cleanup()

	composeYAML := golden.Get(t, "input/simple-single-service.yaml")
	tmpDir := fs.NewDir(t, t.Name(),
		fs.WithFile(composeFileName, "", fs.WithBytes(composeYAML)),
	)
	defer tmpDir.Remove()

	cmd.Command = dockerCli.Command("ecs", "compose", "--file="+tmpDir.Join(composeFileName), "--project-name", t.Name(), "convert")
	result := icmd.RunCmd(cmd).Assert(t, icmd.Success).Combined()

	expected := "simple/simple-cloudformation-conversion.golden"
	golden.Assert(t, result, expected)
}

func TestSimpleWithOverrides(t *testing.T) {
	cmd, cleanup := dockerCli.createTestCmd()
	defer cleanup()

	composeYAML := golden.Get(t, "input/simple-single-service.yaml")
	overriddenComposeYAML := golden.Get(t, "input/simple-single-service-with-overrides.yaml")
	tmpDir := fs.NewDir(t, t.Name(),
		fs.WithFile(composeFileName, "", fs.WithBytes(composeYAML)),
		fs.WithFile("overriddenService.yaml", "", fs.WithBytes(overriddenComposeYAML)),
	)
	defer tmpDir.Remove()
	cmd.Command = dockerCli.Command("ecs", "compose", "--file="+tmpDir.Join(composeFileName), "--file",
		tmpDir.Join("overriddenService.yaml"), "--project-name", t.Name(), "convert")
	result := icmd.RunCmd(cmd).Assert(t, icmd.Success).Combined()

	expected := "simple/simple-cloudformation-with-overrides-conversion.golden"
	golden.Assert(t, result, expected)
}
