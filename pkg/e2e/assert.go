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

	"gotest.tools/v3/poll"
)

// RequireServiceState ensures that the container reaches the expected state
// (running or exited). The daemon reports state transitions asynchronously
// from everything else a test can observe (a container whose logs already
// flowed may still be listed under its previous state for a moment), so the
// check polls `compose ps` until the state converges instead of asserting on
// a single snapshot.
func RequireServiceState(t testing.TB, cli *CLI, service string, state string) {
	t.Helper()
	poll.WaitOn(t, func(poll.LogT) poll.Result {
		// NoCheck: a non-zero `compose ps` is a transient state here (the
		// project may not be registered yet) — and the asserting variant
		// would t.FailNow() from the poll goroutine, which terminates it via
		// runtime.Goexit without reporting: the poll would hang until its
		// opaque timeout instead of surfacing the actual failure below.
		psRes := cli.RunDockerComposeCmdNoCheck(t, "ps", "--all", "--format=json", service)
		if psRes.ExitCode != 0 {
			return poll.Continue("`compose ps %s` exited %d: %s", service, psRes.ExitCode, psRes.Combined())
		}
		out := strings.TrimSpace(psRes.Stdout())
		if out == "" {
			// The container is not registered yet (creation in progress):
			// transient, keep polling.
			return poll.Continue("service %q has no `compose ps` entry yet", service)
		}
		// --format=json emits one JSON object per line, and a service can
		// briefly list two containers mid-transition (the old one being
		// removed, its replacement being created). Succeed as soon as one
		// entry of the target service reaches the expected state; everything
		// short of malformed JSON is a transient condition to retry, not a
		// hard failure — hard-failing on those is the exact race this helper
		// exists to absorb.
		var seen []string
		for line := range strings.SplitSeq(out, "\n") {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return poll.Error(fmt.Errorf("invalid `compose ps` JSON: %w: command output: %s", err, psRes.Combined()))
			}
			if name, _ := entry["Service"].(string); name != service {
				// ps was invoked filtered on the service name; a foreign or
				// incomplete entry is transient noise.
				continue
			}
			current, _ := entry["State"].(string)
			if strings.EqualFold(state, current) {
				return poll.Success()
			}
			seen = append(seen, current)
		}
		return poll.Continue("service %q is %v, expected %q", service, seen, state)
	}, poll.WithTimeout(15*time.Second), poll.WithDelay(200*time.Millisecond))
}
