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

	"github.com/mattn/go-runewidth"
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

// testClock returns a controllable clock starting at a fixed instant.
func testClock() (func() time.Time, *time.Time) {
	now := time.Unix(1_700_000_000, 0)
	return func() time.Time { return now }, &now
}

func newTermWriter(width, height int) (*termWriter, *bytes.Buffer, *time.Time) {
	var buf bytes.Buffer
	clock, now := testClock()
	w := &termWriter{
		out:       &buf,
		info:      &buf,
		tree:      newTaskTree(),
		scr:       screen{out: &buf},
		size:      func() (int, int) { return width, height },
		now:       clock,
		operation: "pull",
	}
	return w, &buf, now
}

// feed applies events at the writer's current clock.
func feed(w *termWriter, events ...api.Resource) {
	for _, e := range events {
		w.tree.apply(e, w.now())
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
	for _, width := range []int{20, 30, 40, 60, 80, 120} {
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
	time.Sleep(20 * time.Millisecond) // let the refresh goroutine exit

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

// The spinner animation must be a function of time, not of how often the
// layout runs.
func TestTerm_SpinnerIsTimeBased(t *testing.T) {
	clock, now := testClock()
	n := &node{status: api.Working, startedAt: clock()}

	a := spinGlyph(n, *now)
	b := spinGlyph(n, *now)
	assert.Equal(t, a.text, b.text, "same instant must yield the same frame")

	*now = now.Add(tickInterval)
	c := spinGlyph(n, *now)
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
	w, buf, now := newTermWriter(50, 24)
	feed(w, api.Resource{ID: "Image docker.io/library/nginx-long-name", Text: "Pulling", Status: api.Working})
	*now = now.Add(2 * time.Second)
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
