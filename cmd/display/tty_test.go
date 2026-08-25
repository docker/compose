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

package display

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jonboulle/clockwork"
	"github.com/mattn/go-runewidth"
	"go.uber.org/goleak"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

// stripAnsi removes ANSI escape codes from a string
func stripAnsi(s string) string {
	var result strings.Builder
	inAnsi := false
	for _, r := range s {
		if r == '\x1b' {
			inAnsi = true
			continue
		}
		if inAnsi {
			// ANSI sequences end with a letter (m, h, l, G, etc.)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inAnsi = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// testClock returns a fake clock starting at a fixed instant; tests advance
// it explicitly instead of sleeping.
func testClock() *clockwork.FakeClock {
	return clockwork.NewFakeClockAt(time.Unix(1_700_000_000, 0))
}

func newTermWriter(width, height int) (*termWriter, *bytes.Buffer, *clockwork.FakeClock) {
	var buf bytes.Buffer
	clock := testClock()
	w := &termWriter{
		out:       &buf,
		info:      &buf,
		tree:      newTaskTree(),
		scr:       screen{out: &buf},
		size:      func() (int, int) { return width, height },
		clock:     clock,
		operation: "pull",
	}
	return w, &buf, clock
}

// feed applies events at the writer's current clock.
func feed(w *termWriter, events ...api.Resource) {
	for _, e := range events {
		w.tree.apply(e, w.clock.Now())
	}
}

// visualLines strips ANSI sequences from the painted frame and returns the
// visible rows.
func visualLines(buf *bytes.Buffer) []string {
	var lines []string
	for _, l := range strings.Split(stripAnsi(buf.String()), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// adversarialEvents exercises every historical overflow trigger at once:
// unbounded status text (skippedEvent), CJK ids, long error details, layers
// with and without totals.
func adversarialEvents() []api.Resource {
	events := []api.Resource{
		{ID: "app", Status: api.Warning, Text: "Skipped: current commandline does not match manifest, and image has no local build metadata"},
		{ID: "Image 测试测试测试测试-with-a-very-long-tag:v1.2.3-alpha.4", Status: api.Working, Text: "Pulling"},
		{ID: "db", Status: api.Error, Text: "Error", Details: "réseau « frontend » introuvable — vérifiez la configuration réseau du projet et les alias déclarés"},
	}
	for i := range 20 {
		events = append(events, api.Resource{
			ID:       fmt.Sprintf("layer-%02d", i),
			ParentID: "Image 测试测试测试测试-with-a-very-long-tag:v1.2.3-alpha.4",
			Status:   api.Working,
			Text:     "Downloading",
			Current:  int64(i) * 1_000_000,
			Total:    50_000_000,
			Percent:  i * 5,
		})
	}
	return events
}

func TestTerm_NoLineEverExceedsTerminalWidth(t *testing.T) {
	// 8 and 12 exercise the degenerate widths where even the "[+] op N/M"
	// header must be clipped rather than allowed to wrap
	for _, width := range []int{8, 12, 20, 30, 40, 60, 80, 120} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			w, buf, _ := newTermWriter(width, 40)
			feed(w, adversarialEvents()...)
			w.repaint()

			for i, line := range visualLines(buf) {
				got := runewidth.StringWidth(strings.TrimRight(line, " "))
				assert.Assert(t, got <= width,
					"line %d is %d cells wide (> %d): %q", i, got, width, line)
			}
		})
	}
}

func TestTerm_TruncationPreservesUTF8(t *testing.T) {
	w, buf, _ := newTermWriter(40, 24)
	feed(w, adversarialEvents()...)
	w.repaint()

	for i, line := range visualLines(buf) {
		assert.Assert(t, utf8.ValidString(line),
			"line %d is invalid UTF-8 after truncation: %q", i, line)
	}
}

// Done must never block, even when the operation context was cancelled
// (Ctrl-C) before Done runs — the historical unbuffered-channel handshake
// deadlocked in that ordering.
func TestTerm_DoneReturnsAfterContextCancel(t *testing.T) {
	w, _, _ := newTermWriter(80, 24)
	ctx, cancel := context.WithCancel(t.Context())
	w.Start(ctx, "pull")
	w.On(api.Resource{ID: "Image foo", Text: "Pulling", Status: api.Working})
	cancel()
	// no settling sleep: Done itself waits for the refresh goroutine to exit

	finished := make(chan struct{})
	go func() {
		w.Done("pull", true)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Done() blocked after context cancellation")
	}
}

// Done is reachable through the public api.EventProcessor interface, so a
// caller isn't guaranteed to have called Start first: it must be a no-op,
// not a nil dereference.
func TestTerm_DoneBeforeStartDoesNotPanic(t *testing.T) {
	w, _, _ := newTermWriter(80, 24)
	w.Done("op", false)
}

// Once Done returns, nothing repaints anymore: the refresh goroutine must
// not survive the bracket, or a stray frame could garble whatever the caller
// prints next (a prompt, container logs). goleak needs no settling sleep
// precisely because Done waits for the goroutine to exit.
func TestTerm_NoRenderGoroutineSurvivesDone(t *testing.T) {
	w, _, _ := newTermWriter(80, 24)
	w.Start(t.Context(), "pull")
	w.On(api.Resource{ID: "Image foo", Text: "Pulling", Status: api.Working})
	w.Done("pull", true)
	goleak.VerifyNone(t)
}

// A Start before the matching Done (nested brackets are a misuse, but Full
// is public API) must retire the previous cycle instead of leaking its
// refresh goroutine until the parent context is cancelled.
func TestTerm_NestedStartRetiresPreviousCycle(t *testing.T) {
	w, _, _ := newTermWriter(80, 24)
	ctx := t.Context()
	w.Start(ctx, "outer")
	w.Start(ctx, "inner")
	w.Done("inner", false)
	w.Done("outer", false)
	goleak.VerifyNone(t)
}

// Build suspension is scoped to a cycle: an operation ending mid-build (a
// failed build aborts the bracket with suspended still set) must not leave
// the next operation's progress permanently silent.
func TestTerm_StartResetsBuildSuspension(t *testing.T) {
	w, buf, _ := newTermWriter(80, 24)
	w.Start(t.Context(), "build")
	w.On(api.Resource{ID: "Image app", Text: api.StatusBuilding, Status: api.Working})
	w.Done("build", false)

	w.Start(t.Context(), "create")
	w.On(api.Resource{ID: "container app-1", Text: "Creating", Status: api.Working})
	w.mu.Lock()
	w.repaint()
	w.mu.Unlock()
	w.Done("create", true)

	frame := stripAnsi(buf.String())
	assert.Assert(t, strings.Contains(frame, "container app-1"), "expected the new cycle to paint, got: %q", frame)
}

// Degenerate terminal heights must never overflow: even height==1 (where
// header + "... more" would naively yield two lines) returns a single line.
func TestTerm_FrameNeverExceedsTerminalHeight(t *testing.T) {
	tree := newTaskTree()
	clock := testClock()
	for _, id := range []string{"a", "b", "c"} {
		tree.apply(api.Resource{ID: id, Text: "Pulling", Status: api.Working}, clock.Now())
	}
	for height := 1; height <= 4; height++ {
		lines := layoutFrame(&tree, "pull", layoutOpts{width: 40, height: height, now: clock.Now()})
		assert.Assert(t, len(lines) <= height, "height %d yielded %d lines", height, len(lines))
	}
}

// roots, children and the completed count are maintained incrementally by
// apply; this locks the bookkeeping across the tricky transitions: a node
// created as root later gaining a parent, and a completed node going back
// to work.
func TestTerm_TreeIncrementalBookkeeping(t *testing.T) {
	tree := newTaskTree()
	clock := testClock()

	tree.apply(api.Resource{ID: "layer", Text: "Waiting", Status: api.Working}, clock.Now())
	assert.Equal(t, len(tree.roots), 1)

	// gaining a first parent demotes the node from the roots ("Image app"
	// has no node of its own yet, so no root remains at this point)
	tree.apply(api.Resource{ID: "layer", ParentID: "Image app", Text: "Downloading", Status: api.Working}, clock.Now())
	assert.Equal(t, len(tree.roots), 0)
	tree.apply(api.Resource{ID: "Image app", Text: "Pulling", Status: api.Working}, clock.Now())
	assert.Equal(t, len(tree.roots), 1)
	assert.Equal(t, tree.roots[0].id, "Image app")
	assert.Equal(t, len(tree.children["Image app"]), 1)

	// completion transitions keep the running counter exact, both ways.
	// "layer" was first seen without a parent, so its anchor is "" and only
	// parentless events update its state (the anchor rule).
	done, total := tree.counts()
	assert.Equal(t, done, 0)
	assert.Equal(t, total, 2)
	tree.apply(api.Resource{ID: "layer", Text: "Pulled", Status: api.Done}, clock.Now())
	done, _ = tree.counts()
	assert.Equal(t, done, 1)
	tree.apply(api.Resource{ID: "layer", Text: "Downloading", Status: api.Working}, clock.Now())
	done, _ = tree.counts()
	assert.Equal(t, done, 0)
}

// The spinner animation must be a function of time, not of how often the
// layout runs.
func TestTerm_SpinnerIsTimeBased(t *testing.T) {
	clock := testClock()
	n := &node{status: api.Working, startedAt: clock.Now()}

	a := spinGlyph(n, clock.Now())
	b := spinGlyph(n, clock.Now())
	assert.Equal(t, a.text, b.text, "same instant must yield the same frame")

	clock.Advance(tickInterval)
	c := spinGlyph(n, clock.Now())
	assert.Assert(t, a.text != c.text, "advancing the clock must advance the frame")
}

func TestTerm_DiffRepaintSkipsUnchangedRows(t *testing.T) {
	var buf bytes.Buffer
	s := screen{out: &buf}
	s.paint([]string{"header", "row a", "row b"}, 80)

	buf.Reset()
	s.paint([]string{"header", "row a CHANGED", "row b"}, 80)
	out := buf.String()
	erases := strings.Count(out, "\x1b[2K")
	assert.Equal(t, 1, erases, "only the changed row should be erased and rewritten, got %d in %q", erases, out)
	assert.Assert(t, !strings.Contains(out, "header"), "unchanged header must not be rewritten")
	assert.Assert(t, strings.Contains(out, "row a CHANGED"))

	buf.Reset()
	s.paint([]string{"header", "row a CHANGED", "row b"}, 80)
	assert.Equal(t, "", buf.String(), "identical frame must write nothing")
}

func TestTerm_ShrinkingFrameBlanksLeftoverRows(t *testing.T) {
	var buf bytes.Buffer
	s := screen{out: &buf}
	s.paint([]string{"header", "row a", "row b"}, 80)

	buf.Reset()
	s.paint([]string{"header", "row a"}, 80)
	assert.Equal(t, 1, strings.Count(buf.String(), "\x1b[2K"),
		"the leftover row must be blanked")
}

func TestTerm_HeightCapAddsMoreMarker(t *testing.T) {
	w, buf, _ := newTermWriter(80, 10)
	var events []api.Resource
	for i := range 30 {
		events = append(events, api.Resource{ID: fmt.Sprintf("service-%02d", i), Text: "Creating", Status: api.Working})
	}
	feed(w, events...)
	w.repaint()

	lines := visualLines(buf)
	assert.Assert(t, len(lines) <= 10, "must not paint more rows than the terminal height, got %d", len(lines))
	assert.Assert(t, strings.Contains(lines[len(lines)-1], "more"),
		"last line should advertise the hidden rows: %q", lines[len(lines)-1])
}

// Child tasks (image layers) are not rendered as rows: they only feed the
// parent's aggregated strip and size counters.
func TestTerm_ChildTasksAggregateIntoParentRow(t *testing.T) {
	w, buf, _ := newTermWriter(100, 40)
	feed(w,
		api.Resource{ID: "Image nginx", Text: "Pulling", Status: api.Working},
		api.Resource{ID: "sha256:aaaa", ParentID: "Image nginx", Text: "Downloading", Status: api.Working, Current: 25_000_000, Total: 100_000_000, Percent: 25},
	)
	w.repaint()

	lines := visualLines(buf)
	assert.Equal(t, 2, len(lines), "header + image only, got %v", lines)
	image := lines[1]
	assert.Assert(t, !strings.Contains(buf.String(), "sha256:aaaa"), "layers must not get their own row")
	assert.Assert(t, strings.Contains(image, "25MB / 100MB"), "image row should aggregate layer sizes: %q", image)
}

// Deterministic visual check with a fixed clock: overall shape, id
// truncation and right-aligned timers.
func TestTerm_VisualSnapshot(t *testing.T) {
	w, buf, clock := newTermWriter(50, 24)
	feed(w, api.Resource{ID: "Image docker.io/library/nginx-long-name", Text: "Pulling", Status: api.Working})
	clock.Advance(2 * time.Second)
	feed(w, api.Resource{ID: "Image docker.io/library/nginx-long-name", Text: "Pulled", Status: api.Done})
	feed(w, api.Resource{ID: "Image docker.io/library/postgres-database", Text: "Pulling", Status: api.Working})
	w.repaint()

	lines := visualLines(buf)
	// identical, character for character, to the legacy renderer's golden
	// output in TestPrintWithDimensions_PulledAndPullingWithLongIDs
	expected := []string{
		"[+] pull 1/2",
		" ✔ Image docker.io/library/nginx-l... Pulled  2.0s",
		" ⠋ Image docker.io/library/postgre... Pulling 0.0s",
	}
	assert.Equal(t, len(expected), len(lines))
	for i := range expected {
		assert.Equal(t, expected[i], strings.TrimRight(lines[i], " "), "line %d", i)
	}
}
