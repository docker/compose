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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/poll"
)

// RequireServiceState ensures that the container is in the expected state
// (running or exited).
func RequireServiceState(t testing.TB, cli *CLI, service string, state string) {
	t.Helper()
	psRes := cli.RunDockerComposeCmd(t, "ps", "--all", "--format=json", service)
	var serviceState map[string]any
	assert.NilError(t, json.Unmarshal([]byte(psRes.Stdout()), &serviceState),
		"Invalid `compose ps` JSON: command output: %s",
		psRes.Combined())

	assert.Assert(t, is.Equal(service, serviceState["Service"]), "Found ps output for unexpected service")
	assert.Assert(t, is.Equal(strings.ToLower(state), strings.ToLower(serviceState["State"].(string))),
		"Service %q (%s) not in expected state",
		service, serviceState["Name"],
	)
}

// RequireEventuallyServiceState polls `compose ps` until the service reaches
// the expected state, instead of checking once.
//
// `compose ps` makes a fresh ContainerList call to the daemon, an entirely
// different channel than a container's log stream: an application log line
// already relayed to a caller is no guarantee the daemon-reported state
// has caught up yet by the time that caller turns around and calls `ps` in a
// new process. Callers observing readiness through a log line (or any signal
// other than this same `ps` state) must poll here rather than check once.
func RequireEventuallyServiceState(t testing.TB, cli *CLI, service string, state string) {
	t.Helper()
	check := func(poll.LogT) poll.Result {
		psRes := cli.RunDockerComposeCmdNoCheck(t, "ps", "--all", "--format=json", service)
		if psRes.ExitCode != 0 {
			return poll.Continue("compose ps exited %d: %s", psRes.ExitCode, psRes.Combined())
		}
		// --format=json prints one JSON object per line (NDJSON), not a
		// single object or array: a scaled service, or a transient window
		// during recreation where the old and new containers are both
		// listed, means more than one line for the requested service — every
		// one of them must reach the expected state, not just the first.
		var found bool
		for _, line := range strings.Split(strings.TrimSpace(psRes.Stdout()), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return poll.Error(fmt.Errorf("invalid `compose ps` JSON line %q: %w", line, err))
			}
			if svc, _ := entry["Service"].(string); !strings.EqualFold(svc, service) {
				continue
			}
			found = true
			if current, _ := entry["State"].(string); !strings.EqualFold(current, state) {
				return poll.Continue("service %q not in state %q yet (got %q): %s", service, state, current, psRes.Stdout())
			}
		}
		if found {
			return poll.Success()
		}
		return poll.Continue("service %q not in state %q yet: %s", service, state, psRes.Stdout())
	}
	poll.WaitOn(t, check, poll.WithDelay(250*time.Millisecond), poll.WithTimeout(15*time.Second))
}
