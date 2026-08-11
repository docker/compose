/*
   Copyright 2026 Docker Compose CLI authors

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

func TestImagesAfterImageRemoved(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14014
	// A running container may reference an image record that no longer exists:
	// under the containerd image store, `up --build` with identical content
	// moves the tag to a new index digest and the daemon drops the old one the
	// container was created from — without compose recreating the container
	// (see #13636). `docker rmi -f` of a running container's image produces
	// the same state deterministically. `compose images` must not fail on it.
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-images-removed"
	const image = "compose-e2e-images-removed:v1"

	cleanup := func() {
		c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--timeout=0")
		c.RunDockerOrExitError(t, "image", "rm", "-f", image)
	}
	cleanup()
	t.Cleanup(cleanup)

	c.RunDockerCmd(t, "pull", "-q", "alpine:3.22")
	c.RunDockerCmd(t, "tag", "alpine:3.22", image)

	c.RunDockerComposeCmd(t, "-f", "./fixtures/images-removed/compose.yaml",
		"--project-name", projectName, "up", "-d")

	// remove the image record while the container keeps running; under the
	// containerd image store this leaves the container referencing an image
	// the daemon can't inspect anymore
	c.RunDockerOrExitError(t, "image", "rm", "-f", image)

	res := c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "images")
	res.Assert(t, icmd.Success)
	assert.Assert(t, strings.Contains(res.Combined(), projectName+"-app-1"), res.Combined())
}
