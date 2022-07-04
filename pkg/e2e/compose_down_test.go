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

func TestDown(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "e2e-down"

	t.Run("no resource to remove", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
		res.Assert(t, icmd.Expected{ExitCode: 0, Err: `No resource found to remove for project "e2e-down"`})
	})
}
