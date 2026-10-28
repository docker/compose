//go:build e2e && !windows

/*
   Copyright 2022 Docker Compose CLI authors

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

func TestUpContainerNameConflict(t *testing.T) {
	s := NewScenario(t, "two services claiming the same container_name must be rejected together, but each must run alone")
	name := s.Project() + "-fixed"
	s.Env("FIXED_NAME="+name).
		Step("up with both services is rejected over the name conflict",
			ComposeCmd("up").MayFail(),
			ExitCode(1),
			OutputContains(`container name "`+name+`" is already in use`)).
		Step("the failed up leaves nothing behind",
			ComposeCmd("down")).
		Step("the first service runs alone under the shared name",
			ComposeCmd("up", "test"),
			ServiceState("test", "exited")).
		Step("down frees the name",
			ComposeCmd("down"),
			ServiceNotCreated("test")).
		Step("the other service runs alone under the shared name",
			ComposeCmd("up", "another_test"),
			ServiceState("another_test", "exited"))
}
