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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeExec(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-exec"

	cmdArgs := func(cmd string, args ...string) []string {
		ret := []string{"--project-directory", "fixtures/simple-composefile", "--project-name", projectName, cmd}
		ret = append(ret, args...)
		return ret
	}

	c.RunDockerComposeCmd(t, cmdArgs("up", "-d")...)

	t.Run("exec true", func(t *testing.T) {
		c.RunDockerComposeCmd(t, cmdArgs("exec", "simple", "/bin/true")...)
	})

	t.Run("exec false", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, cmdArgs("exec", "simple", "/bin/false")...)
		res.Assert(t, icmd.Expected{ExitCode: 1})
	})

	t.Run("exec with env set", func(t *testing.T) {
		res := icmd.RunCmd(c.NewDockerComposeCmd(t, cmdArgs("exec", "-e", "FOO", "simple", "/usr/bin/env")...),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "FOO=BAR")
			})
		res.Assert(t, icmd.Expected{Out: "FOO=BAR"})
	})

	t.Run("exec without env set", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, cmdArgs("exec", "-e", "FOO", "simple", "/usr/bin/env")...)
		assert.Check(t, !strings.Contains(res.Stdout(), "FOO="), res.Combined())
	})
}
