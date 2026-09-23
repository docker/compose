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

package compose

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestShouldFollowStartEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    api.ContainerEvent
		attached []string
		attachTo []string
		want     bool
	}{
		{
			name: "ignores non-start events",
			event: api.ContainerEvent{
				Type:    api.ContainerEventExited,
				Service: "validator",
			},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "ignores services outside explicit attach selection",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "event-bus-validator",
				ID:      "event-bus-validator-1",
			},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "ignores containers already attached unless restarting",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-1",
			},
			attached: []string{"validator-1"},
			attachTo: []string{"validator"},
			want:     false,
		},
		{
			name: "follows restarts for attached service",
			event: api.ContainerEvent{
				Type:       api.ContainerEventStarted,
				Service:    "validator",
				ID:         "validator-1",
				Restarting: true,
			},
			attached: []string{"validator-1"},
			attachTo: []string{"validator"},
			want:     true,
		},
		{
			name: "follows selected service when not already attached",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-2",
			},
			attachTo: []string{"validator"},
			want:     true,
		},
		{
			name: "follows service when no explicit attach filter exists",
			event: api.ContainerEvent{
				Type:    api.ContainerEventStarted,
				Service: "validator",
				ID:      "validator-2",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFollowStartEvent(tt.event, tt.attached, tt.attachTo)
			assert.Equal(t, got, tt.want)
		})
	}
}

// TestIsLateStarter is a follow-up to #14140's on-exit-only fix (glours'
// review on that PR): stopOnFirstExit's own sweep isn't the only path that
// can race a service still climbing the dependency graph on its
// uncancelable context -- a graceful Ctrl+C/SIGTERM teardown does too, and
// arrives with u.isTerminated already true regardless of which path set it.
// isLateStarter must gate on that shared flag, not on which listener
// happened to trigger termination.
func TestIsLateStarter(t *testing.T) {
	tests := []struct {
		name       string
		event      api.ContainerEvent
		terminated bool
		want       bool
	}{
		{
			name:       "a container starting before termination is not a late starter",
			event:      api.ContainerEvent{Type: api.ContainerEventStarted},
			terminated: false,
			want:       false,
		},
		{
			name:       "a non-start event after termination is not a late starter",
			event:      api.ContainerEvent{Type: api.ContainerEventExited},
			terminated: true,
			want:       false,
		},
		{
			name:       "a container starting after termination is a late starter",
			event:      api.ContainerEvent{Type: api.ContainerEventStarted},
			terminated: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLateStarter(tt.event, tt.terminated)
			assert.Equal(t, got, tt.want)
		})
	}
}

// TestAppendErrDropsCancellationAfterShutdown is the #13985 follow-up: once
// our own shutdown has canceled globalCtx (monitor detecting termination,
// SIGINT/SIGTERM, ...), a lingering goroutine (log/attach streaming) racing
// that cancellation reports a context.Canceled error carrying no real
// failure. appendErr must drop it instead of turning a clean exit into a
// non-zero one, while still reporting any other, genuine error.
func TestAppendErrDropsCancellationAfterShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	u := &upSession{globalCtx: ctx}

	u.appendErr(errors.New("boom"))
	assert.Equal(t, len(u.errs), 1)

	cancel()

	u.appendErr(fmt.Errorf("streaming logs: %w", context.Canceled))
	assert.Equal(t, len(u.errs), 1, "a context-canceled error after our own shutdown must be dropped")

	u.appendErr(errors.New("a real, unrelated failure"))
	assert.Equal(t, len(u.errs), 2, "a genuine error occurring after shutdown must still be reported")
}

// TestAppendErrKeepsCancellationBeforeShutdown pins the guard on
// globalCtx.Err(): a context.Canceled error must still be reported if it
// didn't come from our own globalCtx being canceled.
func TestAppendErrKeepsCancellationBeforeShutdown(t *testing.T) {
	u := &upSession{globalCtx: t.Context()}

	u.appendErr(context.Canceled)
	assert.Equal(t, len(u.errs), 1)
}
