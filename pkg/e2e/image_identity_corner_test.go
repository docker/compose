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
	"testing"
)

// TestUpDryRunMissingImage: the dry-run client fakes the pull, so the
// post-pull identity resolution must not inspect the real daemon for an image
// that was never actually pulled — that failed with "No such image".
func TestUpDryRunMissingImage(t *testing.T) {
	NewScenario(t, "dry-run up with a locally-missing image must not resolve it against the real daemon").
		Compose(`
services:
  app:
    image: alpine:3.20
    command: ["sleep", "infinity"]
`).
		Step("make sure the image is not in the local store",
			DockerCmd("rmi", "-f", "alpine:3.20").MayFail()).
		Step("dry-run up succeeds on the faked pull",
			ComposeCmd("--dry-run", "up", "-d"),
			OutputNotContains("No such image"))
}

// TestCreateIdempotentDefaultPlatform locks the invariant that with
// DOCKER_DEFAULT_PLATFORM set to a non-native platform, the digest recorded
// after the pull and the one recomputed from the local store on the next run
// agree, whatever platform each resolution used.
func TestCreateIdempotentDefaultPlatform(t *testing.T) {
	s := NewScenario(t, "with DOCKER_DEFAULT_PLATFORM set to a non-native platform, an unchanged create must not recreate", Serial()).
		Requires(ContainerdImageStore)

	// `create` exercises the same pull/label path as `up` without needing
	// emulation to run the non-native binary
	s.Env("DOCKER_DEFAULT_PLATFORM="+s.NonNativePlatform()).
		Compose(`
services:
  app:
    image: alpine:3.20
    command: ["sleep", "infinity"]
`).
		Defer(DockerCmd("rmi", "-f", "alpine:3.20")).
		Step("start without the image in the local store",
			DockerCmd("rmi", "-f", "alpine:3.20").MayFail()).
		Step("create pulls the image and records its identity",
			ComposeCmd("create")).
		Step("an unchanged create is a no-op",
			ComposeCmd("create"),
			OutputNotContains("Recreate"),
			NotRecreated("app"))
}

// TestCreateIdempotentSharedImageMixedPlatforms: two services share the same
// image, one platform-pinned and one not. The shared summary digest must not
// depend on which service pulled last (post-pull resolutions racing on pull
// completion order made this flaky before the digest was resolved for the
// host default platform), and each service must be labeled with its own
// platform's manifest digest — idempotently across runs.
func TestCreateIdempotentSharedImageMixedPlatforms(t *testing.T) {
	s := NewScenario(t, "two services sharing an image, one platform-pinned, must each keep their own platform's manifest digest", Serial()).
		Requires(ContainerdImageStore)

	s.Env("PINNED_PLATFORM="+s.NonNativePlatform()).
		Compose(`
services:
  native:
    image: alpine:3.19
    command: ["sleep", "infinity"]
  pinned:
    image: alpine:3.19
    platform: ${PINNED_PLATFORM}
    command: ["sleep", "infinity"]
`).
		Defer(DockerCmd("rmi", "-f", "alpine:3.19")).
		Step("start without the image in the local store",
			DockerCmd("rmi", "-f", "alpine:3.19").MayFail()).
		Step("create labels each service with its own platform's manifest digest",
			ComposeCmd("create"),
			LabelSet("native", "com.docker.compose.image"),
			LabelSet("pinned", "com.docker.compose.image"),
			LabelsDistinct("com.docker.compose.image", "native", "pinned")).
		Step("an unchanged create is a no-op and keeps both digests",
			ComposeCmd("create"),
			OutputNotContains("Recreate"),
			NotRecreated("native", "pinned"),
			LabelUnchanged("native", "com.docker.compose.image"),
			LabelUnchanged("pinned", "com.docker.compose.image"))
}

// TestPullRefreshWindowExplicitPull: pull_policy daily/weekly/every_N gates
// `up`, but an explicit `compose pull` is the user's way to force a refresh
// ahead of the window, so it must pull even when the image is fresh.
func TestPullRefreshWindowExplicitPull(t *testing.T) {
	NewScenario(t, "an explicit pull must refresh the image even when the pull_policy window is not due").
		Compose(`
services:
  app:
    image: alpine:3.18
    pull_policy: daily
    command: ["sleep", "infinity"]
`).
		Defer(DockerCmd("rmi", "-f", "alpine:3.18")).
		Step("make the image fresh: the daily window is not due",
			ComposeCmd("pull")).
		Step("an explicit pull refreshes ahead of the window",
			ComposeCmd("pull"),
			OutputContains("Pulled"),
			OutputNotContains("Skipped"))
}
