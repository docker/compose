//go:build !windows

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

func TestNoDepsVolumeFrom(t *testing.T) {
	s := NewScenario(t, "up --no-deps must fail when the container providing volumes_from is gone")
	s.Step("up starts the service and the volume donor",
		ComposeCmd("up", "-d"),
		ServiceState("app", "running"),
		ServiceState("db", "running")).
		Step("up --no-deps reuses the donor container while it exists",
			ComposeCmd("up", "--no-deps", "-d", "app"),
			NotRecreated("app", "db")).
		Step("the donor container is removed behind compose's back",
			DockerCmd("rm", "-f", s.Project()+"-db-1"),
			ServiceNotCreated("db")).
		Step("up --no-deps without the donor container is rejected",
			ComposeCmd("up", "--no-deps", "-d", "app").MayFail(),
			ExitCode(1),
			OutputContains("cannot share volume with service db: container missing"))
}

func TestNoDepsNetworkMode(t *testing.T) {
	s := NewScenario(t, "up --no-deps must fail when the container providing the network namespace is gone")
	s.Step("up starts the service and the namespace donor",
		ComposeCmd("up", "-d"),
		ServiceState("app", "running"),
		ServiceState("db", "running")).
		Step("up --no-deps reuses the donor container while it exists",
			ComposeCmd("up", "--no-deps", "-d", "app"),
			NotRecreated("app", "db")).
		Step("the donor container is removed behind compose's back",
			DockerCmd("rm", "-f", s.Project()+"-db-1"),
			ServiceNotCreated("db")).
		Step("up --no-deps without the donor container is rejected",
			ComposeCmd("up", "--no-deps", "-d", "app").MayFail(),
			ExitCode(1),
			OutputContains("cannot share network namespace with service db: container missing"))
}
