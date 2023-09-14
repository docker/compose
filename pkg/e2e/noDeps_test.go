//go:build !windows
// +build !windows

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
	"fmt"
	"testing"

	"gotest.tools/v3/icmd"
)

func TestNoDepsVolumeFrom(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-no-deps-volume-from"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	c.RunDockerComposeCmd(t, "-f", "fixtures/no-deps/volume-from.yaml", "--project-name", projectName, "up", "-d")

	c.RunDockerComposeCmd(t, "-f", "fixtures/no-deps/volume-from.yaml", "--project-name", projectName, "up", "--no-deps", "-d", "app")

	c.RunDockerCmd(t, "rm", "-f", fmt.Sprintf("%s-db-1", projectName))

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/no-deps/volume-from.yaml", "--project-name", projectName, "up", "--no-deps", "-d", "app")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: "cannot share volume with service db: container missing"})
}

func TestNoDepsNetworkMode(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-no-deps-network-mode"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	c.RunDockerComposeCmd(t, "-f", "fixtures/no-deps/network-mode.yaml", "--project-name", projectName, "up", "-d")

	c.RunDockerComposeCmd(t, "-f", "fixtures/no-deps/network-mode.yaml", "--project-name", projectName, "up", "--no-deps", "-d", "app")

	c.RunDockerCmd(t, "rm", "-f", fmt.Sprintf("%s-db-1", projectName))

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/no-deps/network-mode.yaml", "--project-name", projectName, "up", "--no-deps", "-d", "app")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: "cannot share network namespace with service db: container missing"})
}
