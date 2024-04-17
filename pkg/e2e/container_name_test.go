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
	"testing"

	"gotest.tools/v3/icmd"
)

func TestUpContainerNameConflict(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-container_name_conflict"

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/container_name/compose.yaml", "--project-name", projectName, "up")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: `container name "test" is already in use`})

	c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	c.RunDockerComposeCmd(t, "-f", "fixtures/container_name/compose.yaml", "--project-name", projectName, "up", "test")

	c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	c.RunDockerComposeCmd(t, "-f", "fixtures/container_name/compose.yaml", "--project-name", projectName, "up", "another_test")
}
