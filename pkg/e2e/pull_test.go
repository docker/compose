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

// The pull command's report — Pulled vs Skipped — is the CLI's decision, so
// these scenarios legitimately observe the output.

func TestComposePull(t *testing.T) {
	NewScenario(t, "pull must always pull, whether or not the image is present locally").
		Step("start without the images in the local store",
			ComposeCmd("down", "--rmi", "all")).
		Step("pull fetches every service image",
			ComposeCmd("pull"),
			OutputContains("Image alpine:3.14 Pulled"),
			OutputContains("Image alpine:3.15 Pulled")).
		Step("pull again still pulls: the command's default policy is always",
			ComposeCmd("pull"),
			OutputContains("Image alpine:3.14 Pulled"),
			OutputContains("Image alpine:3.15 Pulled"))
}

func TestPullImagePresentLocally(t *testing.T) {
	NewScenario(t, "with pull_policy missing, a present image is skipped but a :latest tag is still pulled").
		Step("warm the local store",
			ComposeCmd("pull")).
		Step("a second pull skips the pinned tag and refreshes latest",
			ComposeCmd("pull"),
			OutputContains("alpine:3.13.12 Skipped Image is already present locally"),
			OutputContains("alpine:latest Pulled"))
}

func TestPullNoImageNameGiven(t *testing.T) {
	NewScenario(t, "pull must skip a build-only service instead of failing").
		Step("pull reports there is nothing to pull",
			ComposeCmd("pull"),
			OutputContains("Skipped No image to be pulled"))
}

func TestPullFailure(t *testing.T) {
	NewScenario(t, "pull must fail when a service image cannot be pulled").
		Step("pull reports the denied image and fails",
			ComposeCmd("pull").MayFail(),
			ExitCode(1),
			OutputContains("pull access denied for does_not_exists"))
}

func TestPullIgnoreFailures(t *testing.T) {
	NewScenario(t, "pull --ignore-pull-failures must succeed and point at the services to build").
		Step("pull succeeds despite the missing image",
			ComposeCmd("pull", "--ignore-pull-failures"),
			OutputContains("Some service image(s) must be built from source by running:"))
}
