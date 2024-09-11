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

package progress

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"

	"github.com/buger/goterm"
	"github.com/docker/go-units"
	"github.com/morikuni/aec"
)

type ttyWriter struct {
	out             io.Writer
	events          map[string]Event
	eventIDs        []string
	repeated        bool
	numLines        int
	done            chan bool
	mtx             *sync.Mutex
	tailEvents      []string
	dryRun          bool
	skipChildEvents bool
	progressTitle   string
}

func (w *ttyWriter) Start(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.print()
			w.printTailEvents()
			return ctx.Err()
		case <-w.done:
			w.print()
			w.printTailEvents()
			return nil
		case <-ticker.C:
			w.print()
		}
	}
}

func (w *ttyWriter) Stop() {
	w.done <- true
}

func (w *ttyWriter) Event(e Event) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.event(e)
}

func (w *ttyWriter) event(e Event) {
	if !utils.StringContains(w.eventIDs, e.ID) {
		w.eventIDs = append(w.eventIDs, e.ID)
	}
	if _, ok := w.events[e.ID]; ok {
		last := w.events[e.ID]
		switch e.Status {
		case Done, Error, Warning:
			if last.Status != e.Status {
				last.stop()
			}
		case Working:
			last.hasMore()
		}
		last.Status = e.Status
		last.Text = e.Text
		last.StatusText = e.StatusText
		// progress can only go up
		if e.Total > last.Total {
			last.Total = e.Total
		}
		if e.Current > last.Current {
			last.Current = e.Current
		}
		if e.Percent > last.Percent {
			last.Percent = e.Percent
		}
		// allow set/unset of parent, but not swapping otherwise prompt is flickering
		if last.ParentID == "" || e.ParentID == "" {
			last.ParentID = e.ParentID
		}
		w.events[e.ID] = last
	} else {
		e.startTime = time.Now()
		e.spinner = newSpinner()
		if e.Status == Done || e.Status == Error {
			e.stop()
		}
		w.events[e.ID] = e
	}
}

func (w *ttyWriter) Events(events []Event) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, e := range events {
		w.event(e)
	}
}

func (w *ttyWriter) TailMsgf(msg string, args ...interface{}) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	msgWithPrefix := msg
	if w.dryRun {
		msgWithPrefix = strings.TrimSpace(api.DRYRUN_PREFIX + msg)
	}
	w.tailEvents = append(w.tailEvents, fmt.Sprintf(msgWithPrefix, args...))
}

func (w *ttyWriter) printTailEvents() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, msg := range w.tailEvents {
		_, _ = fmt.Fprintln(w.out, msg)
	}
}

func (w *ttyWriter) print() { //nolint:gocyclo
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if len(w.eventIDs) == 0 {
		return
	}
	terminalWidth := goterm.Width()
	b := aec.EmptyBuilder
	for i := 0; i <= w.numLines; i++ {
		b = b.Up(1)
	}
	if !w.repeated {
		b = b.Down(1)
	}
	w.repeated = true
	_, _ = fmt.Fprint(w.out, b.Column(0).ANSI)

	// Hide the cursor while we are printing
	_, _ = fmt.Fprint(w.out, aec.Hide)
	defer func() {
		_, _ = fmt.Fprint(w.out, aec.Show)
	}()

	firstLine := fmt.Sprintf("[+] %s %d/%d", w.progressTitle, numDone(w.events), w.numLines)
	if w.numLines != 0 && numDone(w.events) == w.numLines {
		firstLine = DoneColor(firstLine)
	}
	_, _ = fmt.Fprintln(w.out, firstLine)

	var statusPadding int
	for _, v := range w.eventIDs {
		event := w.events[v]
		l := len(fmt.Sprintf("%s %s", event.ID, event.Text))
		if statusPadding < l {
			statusPadding = l
		}
		if event.ParentID != "" {
			statusPadding -= 2
		}
	}

	if len(w.eventIDs) > goterm.Height()-2 {
		w.skipChildEvents = true
	}
	numLines := 0
	for _, v := range w.eventIDs {
		event := w.events[v]
		if event.ParentID != "" {
			continue
		}
		line := w.lineText(event, "", terminalWidth, statusPadding, w.dryRun)
		_, _ = fmt.Fprint(w.out, line)
		numLines++
		for _, v := range w.eventIDs {
			ev := w.events[v]
			if ev.ParentID == event.ID {
				if w.skipChildEvents {
					continue
				}
				line := w.lineText(ev, "  ", terminalWidth, statusPadding, w.dryRun)
				_, _ = fmt.Fprint(w.out, line)
				numLines++
			}
		}
	}
	for i := numLines; i < w.numLines; i++ {
		if numLines < goterm.Height()-2 {
			_, _ = fmt.Fprintln(w.out, strings.Repeat(" ", terminalWidth))
			numLines++
		}
	}
	w.numLines = numLines
}

func (w *ttyWriter) lineText(event Event, pad string, terminalWidth, statusPadding int, dryRun bool) string {
	endTime := time.Now()
	if event.Status != Working {
		endTime = event.startTime
		if (event.endTime != time.Time{}) {
			endTime = event.endTime
		}
	}
	prefix := ""
	if dryRun {
		prefix = PrefixColor(api.DRYRUN_PREFIX)
	}

	elapsed := endTime.Sub(event.startTime).Seconds()

	var (
		hideDetails bool
		total       int64
		current     int64
		completion  []string
	)

	// only show the aggregated progress while the root operation is in-progress
	if parent := event; parent.Status == Working {
		for _, v := range w.eventIDs {
			child := w.events[v]
			if child.ParentID == parent.ID {
				if child.Status == Working && child.Total == 0 {
					// we don't have totals available for all the child events
					// so don't show the total progress yet
					hideDetails = true
				}
				total += child.Total
				current += child.Current
				completion = append(completion, percentChars[(len(percentChars)-1)*child.Percent/100])
			}
		}
	}

	// don't try to show detailed progress if we don't have any idea
	if total == 0 {
		hideDetails = true
	}

	var txt string
	if len(completion) > 0 {
		var details string
		if !hideDetails {
			details = fmt.Sprintf(" %7s / %-7s", units.HumanSize(float64(current)), units.HumanSize(float64(total)))
		}
		txt = fmt.Sprintf("%s [%s]%s %s",
			event.ID,
			SuccessColor(strings.Join(completion, "")),
			details,
			event.Text,
		)
	} else {
		txt = fmt.Sprintf("%s %s", event.ID, event.Text)
	}
	textLen := len(txt)
	padding := statusPadding - textLen
	if padding < 0 {
		padding = 0
	}
	// calculate the max length for the status text, on errors it
	// is 2-3 lines long and breaks the line formatting
	maxStatusLen := terminalWidth - textLen - statusPadding - 15
	status := event.StatusText
	// in some cases (debugging under VS Code), terminalWidth is set to zero by goterm.Width() ; ensuring we don't tweak strings with negative char index
	if maxStatusLen > 0 && len(status) > maxStatusLen {
		status = status[:maxStatusLen] + "..."
	}
	text := fmt.Sprintf("%s %s%s %s%s %s",
		pad,
		event.Spinner(),
		prefix,
		txt,
		strings.Repeat(" ", padding),
		event.Status.colorFn()(status),
	)
	timer := fmt.Sprintf("%.1fs ", elapsed)
	o := align(text, TimerColor(timer), terminalWidth)

	return o
}

func numDone(events map[string]Event) int {
	i := 0
	for _, e := range events {
		if e.Status != Working {
			i++
		}
	}
	return i
}

func align(l, r string, w int) string {
	ll := lenAnsi(l)
	lr := lenAnsi(r)
	pad := ""
	count := w - ll - lr
	if count > 0 {
		pad = strings.Repeat(" ", count)
	}
	return fmt.Sprintf("%s%s%s\n", l, pad, r)
}

// lenAnsi count of user-perceived characters in ANSI string.
func lenAnsi(s string) int {
	length := 0
	ansiCode := false
	for _, r := range s {
		if r == '\x1b' {
			ansiCode = true
			continue
		}
		if ansiCode && r == 'm' {
			ansiCode = false
			continue
		}
		if !ansiCode {
			length++
		}
	}
	return length
}

var (
	percentChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")
)
