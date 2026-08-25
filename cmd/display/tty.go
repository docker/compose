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
	"sync"
	"time"

	"github.com/buger/goterm"

	"github.com/docker/compose/v5/pkg/api"
)

// Full creates the terminal EventProcessor, built on a model/layout/screen
// split:
//
//   - tty_model.go  — event reducer, pure data, injected clock
//   - tty_layout.go — pure (model, size, now) → lines, every line ≤ width
//   - tty_screen.go — diff-based repaint of the block, single Write per frame
//
// The writer itself only coordinates: a mutex guards the model and the
// screen, and the refresh goroutine is stopped through context cancellation
// so no lifecycle transition can block (Done after Ctrl-C included).
func Full(out io.Writer, info io.Writer, detached bool, opts ...TermOption) api.EventProcessor {
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

type termWriter struct {
	mu       sync.Mutex
	out      io.Writer
	info     io.Writer
	detached bool
	dryRun   bool

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

func (w *termWriter) Start(ctx context.Context, operation string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// A Start before the matching Done (nested brackets are a misuse, but
	// Full is public API): retire the previous cycle so its refresh
	// goroutine doesn't outlive it.
	if w.stopTicks != nil {
		w.stopTicks()
		w.stopTicks = nil
	}
	// Build suspension is scoped to a cycle: a new operation must not stay
	// silent because the previous one ended mid-build, and buildkit wrote
	// below the last frame, so the new cycle starts a fresh block.
	if w.suspended {
		w.suspended = false
		w.scr.reset()
	}
	w.operation = operation
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
		w.suspended = false
		w.scr.reset()
	}

	w.tree.apply(e, w.now())

	if w.operation == "" {
		// outside any operation: degrade to one plain line per event
		_, _ = fmt.Fprintf(w.out, "%s %s %s\n", e.ID, plainEventColor(e.Status)(e.Text), e.Details)
	}
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

func plainEventColor(s api.EventStatus) colorFunc {
	switch s {
	case api.Warning:
		return WarningColor
	case api.Error:
		return ErrorColor
	default:
		return SuccessColor
	}
}
