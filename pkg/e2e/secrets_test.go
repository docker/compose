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

func TestSecretFromEnv(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("compose run", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, "-f", "./fixtures/env-secret/compose.yaml", "run", "foo"),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "SECRET=BAR")
			})
		res.Assert(t, icmd.Expected{Out: "BAR"})
	})
}
