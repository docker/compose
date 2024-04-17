//go:build !windows
// +build !windows

/*
   Copyright 2022 Docker Compose CLI authors

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
)

func TestCascadeStop(t *testing.T) {
	c := NewCLI(t)
	const projectName = "compose-e2e-cascade-stop"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/cascade/compose.yaml", "--project-name", projectName,
		"up", "--abort-on-container-exit")
	assert.Assert(t, strings.Contains(res.Combined(), "exit-1 exited with code 0"), res.Combined())
	// no --exit-code-from, so this is not an error
	assert.Equal(t, res.ExitCode, 0)
}

func TestCascadeFail(t *testing.T) {
	c := NewCLI(t)
	const projectName = "compose-e2e-cascade-fail"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/cascade/compose.yaml", "--project-name", projectName,
		"up", "--abort-on-container-failure")
	assert.Assert(t, strings.Contains(res.Combined(), "exit-1 exited with code 0"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "fail-1 exited with code 111"), res.Combined())
	// failing exit code should be propagated
	assert.Equal(t, res.ExitCode, 111)
}
