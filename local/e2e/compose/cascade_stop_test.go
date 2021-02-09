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

	. "github.com/docker/compose-cli/utils/e2e"
)

func TestCascadeStop(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-logs"

	res := c.RunDockerCmd("compose", "-f", "./fixtures/cascade-stop-test/compose.yaml", "--project-name", projectName, "up", "--abort-on-container-exit")
	res.Assert(t, icmd.Expected{Out: `PING localhost (127.0.0.1)`})
	res.Assert(t, icmd.Expected{Out: `/does_not_exist: No such file or directory`})
	res.Assert(t, icmd.Expected{Out: `should_fail_1 exited with code 1`})
	res.Assert(t, icmd.Expected{Out: `Aborting on container exit...`})
	// FIXME res.Assert(t, icmd.Expected{ExitCode: 1})
}
