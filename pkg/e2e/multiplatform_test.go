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

func TestCreateLocalMultiPlatformImage(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14007
	// With the containerd image store, a local multi-platform image holding the
	// requested non-native variant satisfies the default "missing" pull policy:
	// compose must use it and not try to pull the (unpublished) tag.
	c := NewParallelCLI(t)

	driverStatus := c.RunDockerCmd(t, "info", "--format", "{{json .DriverStatus}}").Stdout()
	if !strings.Contains(driverStatus, "io.containerd.snapshotter.v1") {
		t.Skip("containerd image store not enabled, can't hold a multi-platform image locally")
	}

	// request the non-native platform, so the requested variant can't be the
	// one a platform-less image inspect reports
	requested := "linux/amd64"
	if arch := c.RunDockerCmd(t, "info", "--format", "{{.Architecture}}").Stdout(); strings.Contains(arch, "x86_64") {
		requested = "linux/arm64"
	}

	// the tag deliberately doesn't exist on any registry: resolving the
	// requested platform from the local image is the only way to succeed
	const image = "compose-e2e-multiplatform-local-only:v1"
	const projectName = "e2e-multiplatform-local"

	cleanup := func() {
		c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--timeout=0")
		c.RunDockerOrExitError(t, "image", "rm", image)
	}
	cleanup()
	t.Cleanup(cleanup)

	// store both the native and the requested variants under the local-only tag
	c.RunDockerCmd(t, "pull", "-q", "--platform", "linux/amd64", "alpine:3.22")
	c.RunDockerCmd(t, "pull", "-q", "--platform", "linux/arm64", "alpine:3.22")
	c.RunDockerCmd(t, "tag", "alpine:3.22", image)

	// `create` exercises the same image-resolution/pull-policy path as `up`,
	// without requiring emulation to actually run the non-native binary
	cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/multiplatform/compose.yaml",
		"--project-name", projectName, "create")
	cmd.Env = append(cmd.Env, "REQUESTED_PLATFORM="+requested)
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Success)
	assert.Assert(t, !strings.Contains(res.Combined(), "Pulling"), res.Combined())

	// the created container must be for the requested variant
	platform := strings.TrimSpace(c.RunDockerCmd(t, "inspect", "--format",
		"{{.ImageManifestDescriptor.Platform.OS}}/{{.ImageManifestDescriptor.Platform.Architecture}}",
		projectName+"-repro-1").Stdout())
	assert.Equal(t, requested, platform)
}
