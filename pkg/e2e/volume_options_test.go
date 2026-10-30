//go:build e2e

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
	"time"
)

// bind.propagation and volume.nocopy can be written in a `-v` string, so they
// do not need the Mount API of the engine. Whatever API carries them, the
// engine must apply them: this pins the effect, not the mechanism.
func TestVolumeOptionsAreApplied(t *testing.T) {
	NewScenario(t, "a bind propagation and a volume nocopy must take effect in the container").
		Requires(SharedBindPropagation).
		Step("up starts the service with both options",
			ComposeCmd("up", "-d"),
			Eventually(ServiceState("app", "running"), 10*time.Second)).
		Step("the bind mount has the requested propagation",
			ComposeCmd("ps"),
			BindMountPropagation("app", "/data", "rshared")).
		Step("the volume was not filled with the content of the image",
			ComposeCmd("ps"),
			ExecOutputContains("app", `[ -z "$(ls -A /media)" ] && echo media-not-copied`, "media-not-copied"))
}
