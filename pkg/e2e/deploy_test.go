//go:build !windows

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
	"time"

	"gotest.tools/v3/assert"
)

func TestDeploy(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-deploy"

	reset := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans")
	}
	reset()
	t.Cleanup(reset)

	t.Log("Deploy the application")
	c.RunDockerComposeCmd(t, "-f", "fixtures/deploy/compose.yaml", "--project-name", projectName, "deploy", "-d")

	t.Log("Verify service is running")
	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--format", "json")
	output := res.Stdout()
	assert.Assert(t, strings.Contains(output, "running"), "Expected service to be running, got: %s", output)
}

func TestDeployWait(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-deploy-wait"

	reset := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans")
	}
	reset()
	t.Cleanup(reset)

	t.Log("Deploy the application with --wait")
	timeout := time.After(30 * time.Second)
	done := make(chan bool)
	go func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/deploy/compose.yaml", "--project-name", projectName, "deploy", "--wait")
		assert.Assert(t, strings.Contains(res.Combined(), projectName), "Expected project name in output")
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("deploy --wait did not complete in time")
	case <-done:
		break
	}

	t.Log("Verify service is healthy")
	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--format", "json")
	output := res.Stdout()
	assert.Assert(t, strings.Contains(output, "running"), "Expected service to be running, got: %s", output)
}

func TestDeployBuild(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-deploy-build"

	reset := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans")
	}
	reset()
	t.Cleanup(reset)

	t.Log("Deploy the application with --build")
	c.RunDockerComposeCmd(t, "-f", "fixtures/deploy/compose.yaml", "--project-name", projectName, "deploy", "--build", "-d")

	t.Log("Verify service is running")
	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--format", "json")
	output := res.Stdout()
	assert.Assert(t, strings.Contains(output, "running"), "Expected service to be running, got: %s", output)
}

func TestDeployRemoveOrphans(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-deploy-orphans"

	reset := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans")
	}
	reset()
	t.Cleanup(reset)

	t.Log("Deploy the application")
	c.RunDockerComposeCmd(t, "-f", "fixtures/deploy/compose.yaml", "--project-name", projectName, "deploy", "-d")

	t.Log("Verify service is running")
	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--format", "json")
	output := res.Stdout()
	assert.Assert(t, strings.Contains(output, "running"), "Expected service to be running, got: %s", output)

	t.Log("Deploy with --remove-orphans")
	c.RunDockerComposeCmd(t, "-f", "fixtures/deploy/compose.yaml", "--project-name", projectName, "deploy", "--remove-orphans", "-d")

	t.Log("Verify service is still running")
	res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--format", "json")
	output = res.Stdout()
	assert.Assert(t, strings.Contains(output, "running"), "Expected service to be running, got: %s", output)
}
