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

	"gotest.tools/v3/assert"
)

// TestUpIdempotentContainerdStore reproduces the scenario described in
// https://github.com/docker/compose/pull/13998 : under the containerd image
// store, a multi-platform image pulled by the first `up` gets its
// `com.docker.compose.image` label set from the raw digest returned by
// `image inspect` (the manifest-list/index digest). On the next `up`, the
// image is now local, so compose recomputes the label from the per-platform
// manifest digest instead. Those two digests differ for the very same image,
// so compose believes the image changed and recreates the container even
// though nothing did.
//
// `up` MUST be idempotent: running it twice in a row without any change
// must not recreate any container.
func TestUpIdempotentContainerdStore(t *testing.T) {
	c := NewCLI(t)
	requireContainerdStore(t, c)

	const projectName = "compose-e2e-image-identity-idempotent"
	const image = "alpine:3.20"
	const composeFile = "./fixtures/image-identity/compose.yaml"

	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
		c.RunDockerOrExitError(t, "rmi", "-f", image)
	})

	// Start from an image that was never pulled locally: the first `up`
	// below will pull it, exercising the multi-platform pull path.
	c.RunDockerOrExitError(t, "rmi", "-f", image)

	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d")
	containerID := c.RunDockerCmd(t, "inspect", fmt.Sprintf("%s-app-1", projectName), "-f", "{{.Id}}").Stdout()

	res := c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "up", "-d")
	assert.Check(t, !strings.Contains(res.Combined(), "Recreate"), "second `up` should not recreate anything, got: %s", res.Combined())

	newContainerID := c.RunDockerCmd(t, "inspect", fmt.Sprintf("%s-app-1", projectName), "-f", "{{.Id}}").Stdout()
	assert.Equal(t, containerID, newContainerID, "container should not have been recreated by an idempotent `up`")
}

// requireContainerdStore skips the test unless the docker daemon backing the
// CLI instance uses the containerd image store (`io.containerd.snapshotter.v1`
// driver), which is a prerequisite for the image identity bug this test
// covers.
func requireContainerdStore(t *testing.T, c *CLI) {
	t.Helper()
	res := c.RunDockerCmd(t, "info", "-f", "{{json .DriverStatus}}")
	if !strings.Contains(res.Stdout(), "io.containerd.snapshotter.v1") {
		t.Skip("Skipping test: daemon is not using the containerd image store")
	}
}
