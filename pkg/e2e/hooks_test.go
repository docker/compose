/*
Copyright 2023 Docker Compose CLI authors

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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestPostStartHookInError(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-failure"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/poststart/compose-error.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1})
	assert.Assert(t, strings.Contains(res.Combined(), "Error response from daemon: container"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "is not running"), res.Combined())
}

func TestPostStartHookSuccess(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-success"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/poststart/compose-success.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPreStopHookSuccess(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-success"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPreStopHookInError(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-failure"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/prestop/compose-success.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/prestop/compose-error.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/hooks/prestop/compose-error.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
	res.Assert(t, icmd.Expected{ExitCode: 1})
	assert.Assert(t, strings.Contains(res.Combined(), "sample hook exited with status 127"))
}

func TestPreStopHookSuccessWithPreviousStop(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-pre-stop-success-with-previous-stop"

	t.Cleanup(func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
		res.Assert(t, icmd.Expected{ExitCode: 0})
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	res = c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "stop", "sample")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}

func TestPostStartAndPreStopHook(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "hooks-post-start-and-pre-stop"

	t.Cleanup(func() {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "down", "-v", "--remove-orphans", "-t", "0")
		res.Assert(t, icmd.Expected{ExitCode: 0})
	})

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/hooks/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 0})
}
