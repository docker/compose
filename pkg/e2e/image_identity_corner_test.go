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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

// nonNativePlatform returns a linux platform different from the daemon's.
func nonNativePlatform(t *testing.T, c *CLI) string {
	t.Helper()
	arch := c.RunDockerCmd(t, "info", "--format", "{{.Architecture}}").Stdout()
	if strings.Contains(arch, "x86_64") {
		return "linux/arm64"
	}
	return "linux/amd64"
}

// TestUpDryRunMissingImage: the dry-run client fakes the pull, so the
// post-pull identity resolution must not inspect the real daemon for an image
// that was never actually pulled — that failed with "No such image".
func TestUpDryRunMissingImage(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-identity-dryrun"
	const image = "alpine:3.20"
	const composeFile = "./fixtures/image-identity/compose.yaml"

	t.Cleanup(func() {
		c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--timeout=0")
	})
	c.RunDockerOrExitError(t, "rmi", "-f", image)

	res := c.RunDockerComposeCmdNoCheck(t, "--dry-run", "-f", composeFile, "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Success)
	assert.Check(t, !strings.Contains(res.Combined(), "No such image"), res.Combined())
}

// TestCreateIdempotentDefaultPlatform locks the invariant that with
// DOCKER_DEFAULT_PLATFORM set to a non-native platform, the digest recorded
// after the pull and the one recomputed from the local store on the next run
// agree, whatever platform each resolution used.
func TestCreateIdempotentDefaultPlatform(t *testing.T) {
	c := NewCLI(t)
	requireContainerdStore(t, c)

	const projectName = "compose-e2e-identity-default-platform"
	const image = "alpine:3.20"
	const composeFile = "./fixtures/image-identity/compose.yaml"
	platform := nonNativePlatform(t, c)

	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
		c.RunDockerOrExitError(t, "rmi", "-f", image)
	})
	c.RunDockerOrExitError(t, "rmi", "-f", image)

	create := func() *icmd.Result {
		// `create` exercises the same pull/label path as `up` without needing
		// emulation to run the non-native binary
		cmd := c.NewDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "create")
		cmd.Env = append(cmd.Env, "DOCKER_DEFAULT_PLATFORM="+platform)
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Success)
		return res
	}

	create()
	containerID := c.RunDockerCmd(t, "inspect", fmt.Sprintf("%s-app-1", projectName), "-f", "{{.Id}}").Stdout()

	res := create()
	assert.Check(t, !strings.Contains(res.Combined(), "Recreate"), "second `create` should not recreate anything, got: %s", res.Combined())
	newContainerID := c.RunDockerCmd(t, "inspect", fmt.Sprintf("%s-app-1", projectName), "-f", "{{.Id}}").Stdout()
	assert.Equal(t, containerID, newContainerID)
}

// TestCreateIdempotentSharedImageMixedPlatforms: two services share the same
// image, one platform-pinned and one not. The shared summary digest must not
// depend on which service pulled last (post-pull resolutions racing on pull
// completion order made this flaky before the digest was resolved for the
// host default platform), and each service must be labeled with its own
// platform's manifest digest — idempotently across runs.
func TestCreateIdempotentSharedImageMixedPlatforms(t *testing.T) {
	c := NewCLI(t)
	requireContainerdStore(t, c)

	const projectName = "compose-e2e-identity-mixed-platforms"
	const image = "alpine:3.19"
	const composeFile = "./fixtures/image-identity/mixed-platforms.yaml"
	platform := nonNativePlatform(t, c)

	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
		c.RunDockerOrExitError(t, "rmi", "-f", image)
	})
	c.RunDockerOrExitError(t, "rmi", "-f", image)

	create := func() *icmd.Result {
		cmd := c.NewDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "create")
		cmd.Env = append(cmd.Env, "PINNED_PLATFORM="+platform)
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Success)
		return res
	}

	create()
	label := func(service string) string {
		return strings.TrimSpace(c.RunDockerCmd(t, "inspect",
			fmt.Sprintf("%s-%s-1", projectName, service),
			"-f", `{{index .Config.Labels "com.docker.compose.image"}}`).Stdout())
	}
	nativeDigest, pinnedDigest := label("native"), label("pinned")
	assert.Check(t, nativeDigest != "", "native service must be labeled")
	assert.Check(t, pinnedDigest != "", "pinned service must be labeled")
	assert.Check(t, nativeDigest != pinnedDigest,
		"the two services must be labeled with their own platform's manifest digest")

	res := create()
	assert.Check(t, !strings.Contains(res.Combined(), "Recreate"),
		"second `create` should not recreate anything, got: %s", res.Combined())
	assert.Equal(t, label("native"), nativeDigest)
	assert.Equal(t, label("pinned"), pinnedDigest)
}

// TestPullRefreshWindowExplicitPull: pull_policy daily/weekly/every_N gates
// `up`, but an explicit `compose pull` is the user's way to force a refresh
// ahead of the window, so it must pull even when the image is fresh.
func TestPullRefreshWindowExplicitPull(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-identity-refresh-window"
	const image = "alpine:3.18"
	const composeFile = "./fixtures/image-identity/refresh-window.yaml"

	t.Cleanup(func() {
		c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--timeout=0")
		c.RunDockerOrExitError(t, "rmi", "-f", image)
	})

	// make the image fresh: the window (daily) is not due
	c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "pull")

	res := c.RunDockerComposeCmd(t, "-f", composeFile, "--project-name", projectName, "pull")
	assert.Check(t, strings.Contains(res.Combined(), "Pulled"),
		"explicit pull must refresh ahead of the window, got: %s", res.Combined())
	assert.Check(t, !strings.Contains(res.Combined(), "Skipped"),
		"explicit pull must not skip on the refresh window, got: %s", res.Combined())
}
