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

func TestCreateLocalMultiPlatformImage(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14007
	// With the containerd image store, a local multi-platform image holding the
	// requested non-native variant satisfies the default "missing" pull policy:
	// compose must use it and not try to pull the (unpublished) tag.
	s := NewScenario(t, "a local multi-platform image must satisfy the default missing pull policy for a non-native platform").
		Requires(ContainerdImageStore)

	// request the non-native platform, so the requested variant can't be the
	// one a platform-less image inspect reports; the tag deliberately doesn't
	// exist on any registry: resolving the requested platform from the local
	// image is the only way to succeed
	requested := s.NonNativePlatform()
	const image = "compose-e2e-multiplatform-local-only:v1"

	// `create` exercises the same image-resolution/pull-policy path as `up`,
	// without requiring emulation to actually run the non-native binary
	s.Env("REQUESTED_PLATFORM="+requested).
		Compose(`
services:
  repro:
    image: compose-e2e-multiplatform-local-only:v1
    platform: ${REQUESTED_PLATFORM}
    command: ["uname", "-m"]
`).
		Defer(DockerCmd("image", "rm", image)).
		Step("store the amd64 variant in the local store",
			DockerCmd("pull", "-q", "--platform", "linux/amd64", "alpine:3.22")).
		Step("store the arm64 variant in the local store",
			DockerCmd("pull", "-q", "--platform", "linux/arm64", "alpine:3.22")).
		Step("tag both variants under a tag that exists on no registry",
			DockerCmd("tag", "alpine:3.22", image)).
		Step("create uses the local variant for the requested platform without pulling",
			ComposeCmd("create"),
			OutputNotContains("Pulling"),
			RunsOnPlatform("repro", requested))
}
