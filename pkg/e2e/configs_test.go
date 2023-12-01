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

func TestConfigFromEnv(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("config from file", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, "-f", "./fixtures/configs/compose.yaml", "run", "from_file"))
		res.Assert(t, icmd.Expected{Out: "This is my config file"})
	})

	t.Run("config from env", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, "-f", "./fixtures/configs/compose.yaml", "run", "from_env"),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "CONFIG=config")
			})
		res.Assert(t, icmd.Expected{Out: "config"})
	})

	t.Run("config inlined", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, "-f", "./fixtures/configs/compose.yaml", "run", "inlined"),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "CONFIG=config")
			})
		res.Assert(t, icmd.Expected{Out: "This is my config"})
	})

	t.Run("custom target", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, "-f", "./fixtures/configs/compose.yaml", "run", "target"),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "CONFIG=config")
			})
		res.Assert(t, icmd.Expected{Out: "This is my config"})
	})
}
