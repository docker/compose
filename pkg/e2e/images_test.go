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

func TestImagesAfterImageRemoved(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/14014
	// A running container may reference an image record that no longer exists:
	// under the containerd image store, `up --build` with identical content
	// moves the tag to a new index digest and the daemon drops the old one the
	// container was created from — without compose recreating the container
	// (see #13636). `docker rmi -f` of a running container's image produces
	// the same state deterministically. `compose images` must not fail on it.
	s := NewScenario(t, "images must list containers whose image record is gone")
	image := s.Project() + ":v1"
	s.Env("REMOVED_IMAGE="+image).
		Defer(DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("tag a local image for the service",
			DockerCmd("pull", "-q", "alpine:3.22")).
		Step("with the service's tag",
			DockerCmd("tag", "alpine:3.22", image)).
		Step("up starts the service from the tag",
			ComposeCmd("up", "-d"),
			ServiceState("app", "running")).
		Step("remove the image record while the container keeps running",
			DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("images still lists the project's container",
			ComposeCmd("images"),
			OutputContains(s.Project()+"-app-1"))
}
