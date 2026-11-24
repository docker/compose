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
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/buger/goterm"

	"github.com/docker/compose/v5/pkg/api"
)

// Full creates the terminal display, built on a model/layout/screen split.
//
// It is an [api.StatusRenderer]: given the status of the operation it draws
// from it (see [termWriter.sync]) and the events of the operation are not what
// it is fed. It still is an [api.EventProcessor] for the events it is handed
// outside of an operation, and for a backend that keeps no status.
//
//   - tty_model.go  — state reducer, pure data, injected clock
//   - tty_layout.go — pure (model, size, now) → lines, every line ≤ width
//   - tty_screen.go — diff-based repaint of the block, single Write per frame
//
// The writer itself only coordinates: a mutex guards the model and the
// screen, and the refresh goroutine is stopped through context cancellation
// so no lifecycle transition can block (Done after Ctrl-C included).
func Full(out io.Writer, info io.Writer, detached bool, opts ...TermOption) api.EventProcessor {
	return newTerm(out, info, detached, opts...)
}

func newTerm(out io.Writer, info io.Writer, detached bool, opts ...TermOption) *termWriter {
	w := &termWriter{
		out:      out,
		info:     info,
		detached: detached,
		tree:     newTaskTree(),
		scr:      screen{out: out},
		size:     termSize,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// TermOption customizes the Full EventProcessor.
type TermOption func(*termWriter)

// WithDryRun prefixes every row with the dry-run marker.
func WithDryRun() TermOption {
	return func(w *termWriter) { w.dryRun = true }
}

var _ api.StatusRenderer = (*termWriter)(nil)

type termWriter struct {
	mu       sync.Mutex
	out      io.Writer
	info     io.Writer
	detached bool
	dryRun   bool

	// source is the status the writer draws from, if it has one
	source api.StatusSource
	// seen is the version of the status the tree reflects, and noticesSeen how
	// many of its notices were printed
	seen        uint64
	noticesSeen int

	tree        taskTree
	scr         screen
	operation   string
	suspended   bool
	stopTicks   context.CancelFunc
	ticksExited chan struct{} // closed by the refresh goroutine when it returns

	// injected for tests
	size func() (width, height int)
	now  func() time.Time
}

func termSize() (int, int) {
	width, height := goterm.Width(), goterm.Height()
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

// Attach gives the writer the status it draws from.
func (w *termWriter) Attach(source api.StatusSource) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.source = source
}

func (w *termWriter) Start(ctx context.Context, operation string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// A Start before the matching Done (nested brackets are a misuse, but
	// Full is public API): retire the previous cycle so its refresh
	// goroutine doesn't outlive it. Cancellation only guarantees the old
	// goroutine *will* exit — not that it already has when Start returns;
	// a last repaint from it is harmless (same model, under the mutex).
	if w.stopTicks != nil {
		w.stopTicks()
		w.stopTicks = nil
	}
	// Build suspension is scoped to a cycle: a new operation must not stay
	// silent because the previous one ended mid-build.
	if w.suspended {
		w.unsuspend()
	}
	w.operation = operation
	// what is shown depends on the operation: look at the status again
	w.seen = 0
	// The refresh goroutine is bound to a derived context: parent
	// cancellation (Ctrl-C) and Done both stop it by cancelling, which can
	// never block — there is no channel handshake to miss.
	tickCtx, cancel := context.WithCancel(ctx)
	exited := make(chan struct{})
	w.stopTicks = cancel
	w.ticksExited = exited
	go w.refresh(tickCtx, exited)
}

func (w *termWriter) refresh(ctx context.Context, exited chan<- struct{}) {
	defer close(exited)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			w.sync()
			w.repaint()
			w.mu.Unlock()
		}
	}
}

func (w *termWriter) Done(string, bool) {
	w.mu.Lock()
	if w.stopTicks != nil {
		w.stopTicks()
		w.stopTicks = nil
	}
	exited := w.ticksExited
	w.ticksExited = nil
	w.sync()
	w.repaint() // leave the final state on screen
	w.operation = ""
	// The tree and the screen survive: a follow-up operation on the same
	// writer (`up` chains create and start) extends the same block.
	w.mu.Unlock()

	// Wait for the refresh goroutine outside the lock (it may be blocked on
	// it, about to no-op since operation is cleared): once Done returns,
	// nothing repaints anymore, so output printed right after an operation
	// (a prompt, container logs) cannot be garbled by a stray frame.
	if exited != nil {
		<-exited
	}
}

// sync brings the tree to the current status, and prints the notices raised
// since the last time. Whatever the status says is what is drawn: a resource is
// shown as the plan has yet to reach it, then in the state it reports.
func (w *termWriter) sync() {
	if w.source == nil {
		return
	}
	st := w.source.Status()
	if st.Version == w.seen {
		return
	}
	w.seen = st.Version

	// Buildkit paints its own UI on the same stream: stay silent while an
	// image is being built, and once it is over start a fresh block below
	// whatever buildkit wrote instead of repainting over it.
	building := slices.ContainsFunc(st.Tasks, func(t api.Resource) bool {
		return t.Text == api.StatusBuilding && t.Status == api.Working
	})
	switch {
	case building && !w.suspended:
		w.suspended = true
	case !building && w.suspended:
		w.unsuspend()
	}

	// Tasks, which are what ran before the plan did, then the resources the
	// plan lists in its order.
	for _, t := range st.Tasks {
		w.tree.apply(t, w.now())
	}
	for _, r := range st.Resources {
		if w.hidden(r) {
			continue
		}
		if r.State == api.StateSkipped && w.tree.nodes[r.ID()] == nil {
			// settled without ever having been shown: it was held back, and
			// there is nothing to say about a resource that was not
			continue
		}
		w.tree.apply(api.Resource{ID: r.ID(), Status: r.Level, Text: r.State, Details: r.Message}, w.now())
	}
	for ; w.noticesSeen < len(st.Notices); w.noticesSeen++ {
		_, _ = fmt.Fprintln(w.info, ErrorColor(st.Notices[w.noticesSeen]))
	}
}

// hidden tells whether a resource is left out of the display. Attached run/up
// stream container logs on this same terminal, and painting Starting/Started
// rows there would repaint over the first log lines of the container that just
// started: outside an explicit `start` operation those states are not shown,
// nor the plan to start a container.
func (w *termWriter) hidden(r api.ResourceStatus) bool {
	if w.detached || w.operation == "start" {
		return false
	}
	switch {
	case r.State == api.StatusStarted || r.State == api.StatusStarting:
		return true
	case r.Pending():
		return strings.HasPrefix(r.Message, "start")
	default:
		return false
	}
}

func (w *termWriter) On(events ...api.Resource) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range events {
		if e.ID == api.ResourceCompose {
			_, _ = fmt.Fprintln(w.info, ErrorColor(e.Details))
			continue
		}
		if w.operation != "start" && (e.Text == api.StatusStarted || e.Text == api.StatusStarting) && !w.detached {
			// Deliberate: attached run/up stream container logs on this same
			// terminal, and painting Starting/Started rows here would repaint
			// over the first log lines of the container that just started.
			// Outside an explicit `start` operation those events are dropped.
			continue
		}
		w.handle(e)
	}
}

func (w *termWriter) handle(e api.Resource) {
	// Buildkit paints its own UI on the same stream: stay silent while a
	// build is in flight, and once it's over start a fresh block below
	// whatever buildkit wrote instead of repainting over it.
	if e.Text == api.StatusBuilding {
		w.suspended = true
	} else if w.suspended {
		w.unsuspend()
	}

	w.tree.apply(e, w.now())

	if w.operation == "" {
		// outside any operation: degrade to one plain line per event. This
		// prints outside the tracked block, so the next repaint must not
		// assume the block is still directly above the cursor (same reason
		// unsuspend resets after buildkit output).
		_, _ = fmt.Fprintf(w.out, "%s %s %s\n", e.ID, eventColor(e.Status, SuccessColor)(e.Text), e.Details)
		w.scr.reset()
	}
}

// unsuspend clears build suspension and starts a fresh block below whatever
// buildkit wrote, instead of repainting over it.
func (w *termWriter) unsuspend() {
	w.suspended = false
	w.scr.reset()
}

func (w *termWriter) repaint() {
	if w.suspended || w.operation == "" || len(w.tree.nodes) == 0 {
		return
	}
	width, height := w.size()
	lines := layoutFrame(&w.tree, w.operation, layoutOpts{
		width:  width,
		height: height,
		dryRun: w.dryRun,
		now:    w.now(),
	})
	w.scr.paint(lines, width)
}
