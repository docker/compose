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
	"fmt"
	"strings"
	"testing"

	testify "github.com/stretchr/testify/assert"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

const NO_STATE_TO_CHECK = ""

func TestScaleBasicCases(t *testing.T) {
	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=scale-basic-tests"))

	reset := func() {
		c.RunDockerComposeCmd(t, "down", "--rmi", "all")
	}
	t.Cleanup(reset)
	res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d")
	res.Assert(t, icmd.Success)

	t.Log("scale up one service")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "dbadmin=2")
	out := res.Combined()
	checkServiceContainer(t, out, "scale-basic-tests-dbadmin", "Started", 2)

	t.Log("scale up 2 services")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "front=3", "back=2")
	out = res.Combined()
	checkServiceContainer(t, out, "scale-basic-tests-front", "Running", 2)
	checkServiceContainer(t, out, "scale-basic-tests-front", "Started", 1)
	checkServiceContainer(t, out, "scale-basic-tests-back", "Running", 1)
	checkServiceContainer(t, out, "scale-basic-tests-back", "Started", 1)

	t.Log("scale down one service")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "dbadmin=1")
	out = res.Combined()
	checkServiceContainer(t, out, "scale-basic-tests-dbadmin", "Running", 1)

	t.Log("scale to 0 a service")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "dbadmin=0")
	assert.Check(t, res.Stdout() == "", res.Stdout())

	t.Log("scale down 2 services")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "front=2", "back=1")
	out = res.Combined()
	checkServiceContainer(t, out, "scale-basic-tests-front", "Running", 2)
	assert.Check(t, !strings.Contains(out, "Container scale-basic-tests-front-3  Running"), res.Combined())
	checkServiceContainer(t, out, "scale-basic-tests-back", "Running", 1)
}

func TestScaleWithDepsCases(t *testing.T) {
	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=scale-deps-tests"))

	reset := func() {
		c.RunDockerComposeCmd(t, "down", "--rmi", "all")
	}
	t.Cleanup(reset)
	res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=2")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps")
	checkServiceContainer(t, res.Combined(), "scale-deps-tests-db", NO_STATE_TO_CHECK, 2)

	t.Log("scale up 1 service with --no-deps")
	_ = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "--no-deps", "back=2")
	res = c.RunDockerComposeCmd(t, "ps")
	checkServiceContainer(t, res.Combined(), "scale-deps-tests-back", NO_STATE_TO_CHECK, 2)
	checkServiceContainer(t, res.Combined(), "scale-deps-tests-db", NO_STATE_TO_CHECK, 2)

	t.Log("scale up 1 service without --no-deps")
	_ = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "scale", "back=2")
	res = c.RunDockerComposeCmd(t, "ps")
	checkServiceContainer(t, res.Combined(), "scale-deps-tests-back", NO_STATE_TO_CHECK, 2)
	checkServiceContainer(t, res.Combined(), "scale-deps-tests-db", NO_STATE_TO_CHECK, 1)
}

func TestScaleUpAndDownPreserveContainerNumber(t *testing.T) {
	const projectName = "scale-up-down-test"

	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME="+projectName))

	reset := func() {
		c.RunDockerComposeCmd(t, "down", "--rmi", "all")
	}
	t.Cleanup(reset)
	res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=2", "db")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1\n"+projectName+"-db-2")

	t.Log("scale down removes replica #2")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=1", "db")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1")

	t.Log("scale up restores replica #2")
	res = c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=2", "db")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1\n"+projectName+"-db-2")
}

func TestScaleDownRemovesObsolete(t *testing.T) {
	const projectName = "scale-down-obsolete-test"
	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME="+projectName))

	reset := func() {
		c.RunDockerComposeCmd(t, "down", "--rmi", "all")
	}
	t.Cleanup(reset)
	res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "db")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1")

	cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=2", "db")
	res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
		cmd.Env = append(cmd.Env, "MAYBE=value")
	})
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1\n"+projectName+"-db-2")

	t.Log("scale down removes obsolete replica #1")
	cmd = c.NewDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "db=1", "db")
	res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
		cmd.Env = append(cmd.Env, "MAYBE=value")
	})
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "db")
	res.Assert(t, icmd.Success)
	assert.Equal(t, strings.TrimSpace(res.Stdout()), projectName+"-db-1")
}

func checkServiceContainer(t *testing.T, stdout, containerName, containerState string, count int) {
	found := 0
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.Contains(line, containerName) && strings.Contains(line, containerState) {
			found++
		}
	}
	if found == count {
		return
	}
	errMessage := fmt.Sprintf("expected %d but found %d instance(s) of container %s in stoud", count, found, containerName)
	if containerState != "" {
		errMessage += fmt.Sprintf(" with expected state %s", containerState)
	}
	testify.Fail(t, errMessage, stdout)
}

func TestScaleDownNoRecreate(t *testing.T) {
	const projectName = "scale-down-recreated-test"
	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME="+projectName))

	reset := func() {
		c.RunDockerComposeCmd(t, "down", "--rmi", "all")
	}
	t.Cleanup(reset)
	c.RunDockerComposeCmd(t, "-f", "fixtures/scale/build.yaml", "build", "--build-arg", "FOO=test")
	c.RunDockerComposeCmd(t, "-f", "fixtures/scale/build.yaml", "up", "-d", "--scale", "test=2")

	c.RunDockerComposeCmd(t, "-f", "fixtures/scale/build.yaml", "build", "--build-arg", "FOO=updated")
	c.RunDockerComposeCmd(t, "-f", "fixtures/scale/build.yaml", "up", "-d", "--scale", "test=4", "--no-recreate")

	res := c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "test")
	res.Assert(t, icmd.Success)
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-1"))
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-2"))
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-3"))
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-4"))

	t.Log("scale down removes obsolete replica #1 and #2")
	c.NewDockerComposeCmd(t, "--project-directory", "fixtures/scale", "up", "-d", "--scale", "test=2")

	res = c.RunDockerComposeCmd(t, "ps", "--format", "{{.Name}}", "test")
	res.Assert(t, icmd.Success)
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-3"))
	assert.Check(t, strings.Contains(res.Stdout(), "scale-down-recreated-test-test-4"))
}
