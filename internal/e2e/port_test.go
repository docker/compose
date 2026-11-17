//go:build e2e && !windows

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

// TestPortIgnoresExposedButUnpublishedPorts locks
// https://github.com/docker/compose/issues/14522: a port the image exposes
// without the service publishing it used to be printed as ":0" with exit 0,
// where `docker port` reports it as not published.
func TestPortIgnoresExposedButUnpublishedPorts(t *testing.T) {
	NewScenario(t, "port must not report a port that is exposed but not published").
		Step("up starts a service that publishes port 80 and one that only exposes it",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running"),
			ServiceState("db", "running")).
		Step("the published port is printed",
			ComposeCmd("port", "web", "80"),
			OutputMatches(`:\d+`)).
		Step("the exposed port is not found",
			ComposeCmd("port", "db", "80").MayFail(),
			ExitCode(1),
			StderrContains("no port 80/tcp for container")).
		Step("listing the mappings leaves it out",
			ComposeCmd("port", "db"),
			StdoutEmpty())
}
