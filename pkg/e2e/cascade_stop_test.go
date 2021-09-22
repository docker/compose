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
	"testing"

	"gotest.tools/v3/icmd"
)

func TestCascadeStop(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "e2e-cascade-stop"

	t.Run("abort-on-container-exit", func(t *testing.T) {
		res := c.RunDockerOrExitError("compose", "-f", "./fixtures/cascade-stop-test/compose.yaml", "--project-name", projectName, "up", "--abort-on-container-exit")
		res.Assert(t, icmd.Expected{ExitCode: 1, Out: `should_fail-1 exited with code 1`})
		res.Assert(t, icmd.Expected{ExitCode: 1, Out: `Aborting on container exit...`})
	})

	t.Run("exit-code-from", func(t *testing.T) {
		res := c.RunDockerOrExitError("compose", "-f", "./fixtures/cascade-stop-test/compose.yaml", "--project-name", projectName, "up", "--exit-code-from=sleep")
		res.Assert(t, icmd.Expected{ExitCode: 137, Out: `should_fail-1 exited with code 1`})
		res.Assert(t, icmd.Expected{ExitCode: 137, Out: `Aborting on container exit...`})
	})

	t.Run("exit-code-from unknown", func(t *testing.T) {
		res := c.RunDockerOrExitError("compose", "-f", "./fixtures/cascade-stop-test/compose.yaml", "--project-name", projectName, "up", "--exit-code-from=unknown")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `no such service: unknown`})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
}
