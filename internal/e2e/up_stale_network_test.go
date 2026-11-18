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

import "testing"

// TestUpRecreatesStoppedContainerOfRemovedNetwork locks docker/compose#14516:
// when the network of a stopped container was removed, up created the network
// again but started the old container, which still refers to the removed one,
// and the engine refused to start it ("network <id> not found").
func TestUpRecreatesStoppedContainerOfRemovedNetwork(t *testing.T) {
	s := NewScenario(t, "up must not start a stopped container on a network that no longer exists")
	s.Step("up starts the service",
		ComposeCmd("up", "-d"),
		ServiceState("app", "running")).
		Step("stop leaves the container on the project network",
			ComposeCmd("stop"),
			ServiceState("app", "exited")).
		Step("the network is removed behind Compose's back",
			DockerCmd("network", "rm", s.Project()+"_default")).
		Step("up creates the network again and replaces the container that referred to the old one",
			ComposeCmd("up", "-d"),
			ExitCode(0),
			ServiceState("app", "running"),
			Recreated("app"))
}

// TestUpStartsCreatedContainerWithoutRecreatingIt guards the fix of
// docker/compose#14516: a container that was created but never started has no
// network ID yet, which must not be taken for a connection to another network.
func TestUpStartsCreatedContainerWithoutRecreatingIt(t *testing.T) {
	NewScenario(t, "up must start a created container as it is, not recreate it").
		Step("create makes the container without starting it",
			ComposeCmd("create"),
			ServiceState("app", "created")).
		Step("up starts the same container",
			ComposeCmd("up", "-d"),
			ServiceState("app", "running"),
			NotRecreated("app"))
}
