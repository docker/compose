//go:build e2e

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
	"time"
)

func TestEventsUnknownService(t *testing.T) {
	NewScenario(t, "events must refuse a service that is not in the project").
		Step("events on an unknown service fails and names it",
			ComposeCmd("events", "not-a-service").MayFail().Within(10*time.Second),
			ExitCode(1),
			StderrContains("no such service: not-a-service"))
}
