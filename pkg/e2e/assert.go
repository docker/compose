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
	var last map[string]any
	check := func(poll.LogT) poll.Result {
		psRes := cli.RunDockerComposeCmdNoCheck(t, "ps", "--all", "--format=json", service)
		var serviceState map[string]any
		if err := json.Unmarshal([]byte(psRes.Stdout()), &serviceState); err != nil {
			return poll.Continue("invalid `compose ps` JSON: command output: %s", psRes.Combined())
		}
		last = serviceState
		current, _ := serviceState["State"].(string)
		if !strings.EqualFold(current, state) {
			return poll.Continue("service %q not in state %q yet (got %q)", service, state, current)
		}
		return poll.Success()
	}
	poll.WaitOn(t, check, poll.WithDelay(250*time.Millisecond), poll.WithTimeout(15*time.Second))

	assert.Assert(t, is.Equal(service, last["Service"]), "Found ps output for unexpected service")
}
