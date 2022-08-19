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

func TestRecreateWithNoDeps(t *testing.T) {
	c := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=recreate-no-deps",
	))

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/dependencies/recreate-no-deps.yaml", "up", "-d")
	res.Assert(t, icmd.Success)

	res = c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/dependencies/recreate-no-deps.yaml", "up", "-d", "--force-recreate", "--no-deps", "my-service")
	res.Assert(t, icmd.Success)

	RequireServiceState(t, c, "my-service", "running")

	c.RunDockerComposeCmd(t, "down")
}
