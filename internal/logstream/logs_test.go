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

package logstream

import (
	"context"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	compose "github.com/docker/compose/v5/pkg/api"
)

// TestRunEndTrackerAnchorsOnPreviousRun pins the re-attach anchor against the
// fast-run race seen in CI: with events delivered in order, the anchor
// captured at start-event time is the PREVIOUS run's end — even when the new
// run exits (and is observed) before the log stream is actually opened.
func TestRunEndTrackerAnchorsOnPreviousRun(t *testing.T) {
	tr := NewRunEndTracker()

	// First start: no previous exit observed → no anchor (caller falls back
	// to the inspected FinishedAt).
	assert.Equal(t, tr.Since("c1"), "")

	// Run N exits at t=1_000_000_001ns, run N+1 starts: the anchor captured
	// at start-event time is run N's end, nanosecond-precise.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c1", Time: 1_000_000_001})
	anchor := tr.Since("c1")
	assert.Equal(t, anchor, "1970-01-01T00:00:01.000000001Z")

	// Run N+1 is fast: its own exit is observed before the log stream opens.
	// The anchor captured above must NOT move — reading it after this point
	// would exclude everything run N+1 printed.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c1", Time: 2_000_000_002})
	assert.Equal(t, anchor, "1970-01-01T00:00:01.000000001Z")
	// The NEXT start anchors on run N+1's end.
	assert.Equal(t, tr.Since("c1"), "1970-01-01T00:00:02.000000002Z")

	// Non-exit events and other containers do not pollute the anchor.
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventStarted, ID: "c1", Time: 9_000_000_000})
	tr.Observe(compose.ContainerEvent{Type: compose.ContainerEventExited, ID: "c2", Time: 3_000_000_003})
	assert.Equal(t, tr.Since("c1"), "1970-01-01T00:00:02.000000002Z")
}

// TestLogCursors_Turns checks the hand-over between the streams of a
// container: in the order they were queued, whichever goroutine gets there
// first; and a stream that gives up (canceled context, error, panic) never
// holds the others back.
func TestLogCursors_Turns(t *testing.T) {
	cursors := NewCursors()
	first, second, third := cursors.Enter("c1"), cursors.Enter("c1"), cursors.Enter("c1")
	other := cursors.Enter("c2")

	granted := make(chan string, 3)
	var wg sync.WaitGroup
	for name, turn := range map[string]*Turn{"third": third, "second": second} { // later turns ask first
		wg.Go(func() {
			assert.NilError(t, turn.Wait(t.Context()))
			granted <- name
			turn.End()
		})
	}

	assert.NilError(t, other.Wait(t.Context()), "another container's streams are independent")
	select {
	case name := <-granted:
		t.Fatalf("%s got its turn before the first stream ended", name)
	case <-time.After(50 * time.Millisecond):
	}

	assert.NilError(t, first.Wait(t.Context()))
	first.End()
	first.End() // harmless
	wg.Wait()
	assert.Equal(t, <-granted, "second")
	assert.Equal(t, <-granted, "third")
}

func TestLogCursors_CanceledWaiterReleasesNext(t *testing.T) {
	cursors := NewCursors()
	first, second, third := cursors.Enter("c1"), cursors.Enter("c1"), cursors.Enter("c1")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, second.Wait(ctx), context.Canceled)
	second.End() // what the stream's defer does

	first.End()
	assert.NilError(t, third.Wait(t.Context()))
}
