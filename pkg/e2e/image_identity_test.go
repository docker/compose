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
	"testing"
)

func TestUpIdempotentContainerdStore(t *testing.T) {
	// Regression test for https://github.com/docker/compose/pull/13998 : under
	// the containerd image store, a multi-platform image pulled by the first
	// `up` gets its `com.docker.compose.image` label set from the raw digest
	// returned by `image inspect` (the manifest-list/index digest). On the
	// next `up`, the image is now local, so compose recomputes the label from
	// the per-platform manifest digest instead. Those two digests differ for
	// the very same image, so compose believed the image changed and recreated
	// the container even though nothing did.
	NewScenario(t, "up must be idempotent for a multi-platform image pulled by the first up", Serial()).
		Requires(ContainerdImageStore).
		Compose(`
services:
  app:
    image: alpine:3.20
    command: ["sleep", "infinity"]
`).
		Defer(DockerCmd("rmi", "-f", "alpine:3.20").MayFail()).
		Step("start from an image that was never pulled locally",
			DockerCmd("rmi", "-f", "alpine:3.20").MayFail()).
		Step("the first up pulls the image and starts the service",
			ComposeCmd("up", "-d"),
			ServiceState("app", "running")).
		Step("a second up changes nothing",
			ComposeCmd("up", "-d"),
			OutputNotContains("Recreate"),
			NotRecreated("app"))
}
