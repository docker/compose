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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/icmd"
)

func TestIPC(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "ipc_e2e"
	var cid string
	t.Run("create ipc mode container", func(t *testing.T) {
		res := c.RunDockerCmd("run", "-d", "--rm", "--ipc=shareable", "--name", "ipc_mode_container", "alpine", "top")
		cid = strings.Trim(res.Stdout(), "\n")
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/ipc-test/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check running project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `shareable`})
	})

	t.Run("check ipcmode in container inspect", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"-shareable-1")
		res.Assert(t, icmd.Expected{Out: `"IpcMode": "shareable",`})

		res = c.RunDockerCmd("inspect", projectName+"-service-1")
		res.Assert(t, icmd.Expected{Out: `"IpcMode": "container:`})

		res = c.RunDockerCmd("inspect", projectName+"-container-1")
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf(`"IpcMode": "container:%s",`, cid)})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
	t.Run("remove ipc mode container", func(t *testing.T) {
		_ = c.RunDockerCmd("rm", "-f", "ipc_mode_container")
	})
}
