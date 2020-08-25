/*
   Copyright 2020 Docker, Inc.

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
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/tests/framework"
)

var binDir string

func TestMain(m *testing.M) {
	p, cleanup, err := SetupExistingCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	binDir = p
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestLocalBackend(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	c.RunDockerCmd("context", "create", "local", "test-context").Assert(t, icmd.Success)
	c.RunDockerCmd("context", "use", "test-context").Assert(t, icmd.Success)

	t.Run("run", func(t *testing.T) {
		res := c.RunDockerCmd("run", "-d", "nginx")
		containerName := strings.TrimSpace(res.Combined())
		t.Cleanup(func() {
			_ = c.RunDockerOrExitError("rm", "-f", containerName)
		})
		res = c.RunDockerCmd("inspect", containerName)
		res.Assert(t, icmd.Expected{Out: `"Status": "running"`})
	})

	t.Run("run with ports", func(t *testing.T) {
		res := c.RunDockerCmd("run", "-d", "-p", "8080:80", "nginx")
		containerName := strings.TrimSpace(res.Combined())
		t.Cleanup(func() {
			_ = c.RunDockerOrExitError("rm", "-f", containerName)
		})
		res = c.RunDockerCmd("inspect", containerName)
		res.Assert(t, icmd.Expected{Out: `"Status": "running"`})
		res = c.RunDockerCmd("ps")
		res.Assert(t, icmd.Expected{Out: "0.0.0.0:8080->80/tcp"})
	})

	t.Run("inspect not found", func(t *testing.T) {
		res := c.RunDockerOrExitError("inspect", "nonexistentcontainer")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "Error: No such container: nonexistentcontainer",
		})
	})
}
