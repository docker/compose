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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/docker/compose/v2/pkg/api"

	"github.com/buger/goterm"
	"github.com/docker/go-units"
	"github.com/morikuni/aec"
)

type ttyWriter struct {
	out             io.Writer
	tasks           map[string]task
	ids             []string
	repeated        bool
	numLines        int
	done            chan bool
	mtx             *sync.Mutex
	tailEvents      []string
	dryRun          bool
	skipChildEvents bool
	progressTitle   string
}

type task struct {
	ID         string
	parentID   string
	startTime  time.Time
	endTime    time.Time
	text       string
	status     EventStatus
	statusText string
	current    int64
	percent    int
	total      int64
	spinner    *Spinner
}

func (t *task) stop() {
	t.endTime = time.Now()
	t.spinner.Stop()
}

func (t *task) hasMore() {
	t.spinner.Restart()
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
	if !slices.Contains(w.ids, e.ID) {
		w.ids = append(w.ids, e.ID)
	}
	if _, ok := w.tasks[e.ID]; ok {
		last := w.tasks[e.ID]
		switch e.Status {
		case Done, Error, Warning:
			if last.status != e.Status {
				last.stop()
			}
		case Working:
			last.hasMore()
		}
		last.status = e.Status
		last.text = e.Text
		last.statusText = e.StatusText
		// progress can only go up
		if e.Total > last.total {
			last.total = e.Total
		}
		if e.Current > last.current {
			last.current = e.Current
		}
		if e.Percent > last.percent {
			last.percent = e.Percent
		}
		// allow set/unset of parent, but not swapping otherwise prompt is flickering
		if last.parentID == "" || e.ParentID == "" {
			last.parentID = e.ParentID
		}
		w.tasks[e.ID] = last
	} else {
		t := task{
			ID:         e.ID,
			parentID:   e.ParentID,
			startTime:  time.Now(),
			text:       e.Text,
			status:     e.Status,
			statusText: e.StatusText,
			current:    e.Current,
			percent:    e.Percent,
			total:      e.Total,
			spinner:    NewSpinner(),
		}
		if e.Status == Done || e.Status == Error {
			t.stop()
		}
		w.tasks[e.ID] = t
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
	if len(w.ids) == 0 {
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

	firstLine := fmt.Sprintf("[+] %s %d/%d", w.progressTitle, numDone(w.tasks), len(w.tasks))
	if w.numLines != 0 && numDone(w.tasks) == w.numLines {
		firstLine = DoneColor(firstLine)
	}
	_, _ = fmt.Fprintln(w.out, firstLine)

	var statusPadding int
	for _, v := range w.ids {
		t := w.tasks[v]
		l := len(fmt.Sprintf("%s %s", t.ID, t.text))
		if statusPadding < l {
			statusPadding = l
		}
		if t.parentID != "" {
			statusPadding -= 2
		}
	}

	if len(w.ids) > goterm.Height()-2 {
		w.skipChildEvents = true
	}
	numLines := 0
	for _, v := range w.ids {
		t := w.tasks[v]
		if t.parentID != "" {
			continue
		}
		line := w.lineText(t, "", terminalWidth, statusPadding, w.dryRun)
		_, _ = fmt.Fprint(w.out, line)
		numLines++
		for _, v := range w.ids {
			t := w.tasks[v]
			if t.parentID == t.ID {
				if w.skipChildEvents {
					continue
				}
				line := w.lineText(t, "  ", terminalWidth, statusPadding, w.dryRun)
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

func (w *ttyWriter) lineText(t task, pad string, terminalWidth, statusPadding int, dryRun bool) string {
	endTime := time.Now()
	if t.status != Working {
		endTime = t.startTime
		if (t.endTime != time.Time{}) {
			endTime = t.endTime
		}
	}
	prefix := ""
	if dryRun {
		prefix = PrefixColor(api.DRYRUN_PREFIX)
	}

	elapsed := endTime.Sub(t.startTime).Seconds()

	var (
		hideDetails bool
		total       int64
		current     int64
		completion  []string
	)

	// only show the aggregated progress while the root operation is in-progress
	if parent := t; parent.status == Working {
		for _, v := range w.ids {
			child := w.tasks[v]
			if child.parentID == parent.ID {
				if child.status == Working && child.total == 0 {
					// we don't have totals available for all the child events
					// so don't show the total progress yet
					hideDetails = true
				}
				total += child.total
				current += child.current
				completion = append(completion, percentChars[(len(percentChars)-1)*child.percent/100])
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
			t.ID,
			SuccessColor(strings.Join(completion, "")),
			details,
			t.text,
		)
	} else {
		txt = fmt.Sprintf("%s %s", t.ID, t.text)
	}
	textLen := len(txt)
	padding := statusPadding - textLen
	if padding < 0 {
		padding = 0
	}
	// calculate the max length for the status text, on errors it
	// is 2-3 lines long and breaks the line formatting
	maxStatusLen := terminalWidth - textLen - statusPadding - 15
	status := t.statusText
	// in some cases (debugging under VS Code), terminalWidth is set to zero by goterm.Width() ; ensuring we don't tweak strings with negative char index
	if maxStatusLen > 0 && len(status) > maxStatusLen {
		status = status[:maxStatusLen] + "..."
	}
	text := fmt.Sprintf("%s %s%s %s%s %s",
		pad,
		spinner(t),
		prefix,
		txt,
		strings.Repeat(" ", padding),
		colorFn(t.status)(status),
	)
	timer := fmt.Sprintf("%.1fs ", elapsed)
	o := align(text, TimerColor(timer), terminalWidth)

	return o
}

var (
	spinnerDone    = "✔"
	spinnerWarning = "!"
	spinnerError   = "✘"
)

func spinner(t task) string {
	switch t.status {
	case Done:
		return SuccessColor(spinnerDone)
	case Warning:
		return WarningColor(spinnerWarning)
	case Error:
		return ErrorColor(spinnerError)
	default:
		return CountColor(t.spinner.String())
	}
}

func colorFn(s EventStatus) colorFunc {
	switch s {
	case Done:
		return SuccessColor
	case Warning:
		return WarningColor
	case Error:
		return ErrorColor
	default:
		return nocolor
	}
}

func numDone(tasks map[string]task) int {
	i := 0
	for _, t := range tasks {
		if t.status != Working {
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

var percentChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")
