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
	"testing"

	"gotest.tools/v3/icmd"
)

func TestExec(t *testing.T) {
	const projectName = "e2e-exec"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	c.RunDockerComposeCmd(t, "-f", "./fixtures/exec/compose.yaml", "--project-name", projectName, "run", "-d", "test", "cat")

	res := c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "exec", "--index=1", "test", "ps")
	res.Assert(t, icmd.Expected{Err: "service \"test\" is not running container #1", ExitCode: 1})

	res = c.RunDockerComposeCmd(t, "--project-name", projectName, "exec", "test", "ps")
	res.Assert(t, icmd.Expected{Out: "cat"}) // one-off container was selected

	c.RunDockerComposeCmd(t, "-f", "./fixtures/exec/compose.yaml", "--project-name", projectName, "up", "-d")

	res = c.RunDockerComposeCmd(t, "--project-name", projectName, "exec", "test", "ps")
	res.Assert(t, icmd.Expected{Out: "tail"}) // service container was selected
}
