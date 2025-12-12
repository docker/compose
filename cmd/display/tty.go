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
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/buger/goterm"
	"github.com/docker/compose/v5/pkg/utils"
	"github.com/docker/go-units"
	"github.com/morikuni/aec"

	"github.com/docker/compose/v5/pkg/api"
)

// Full creates an EventProcessor that render advanced UI within a terminal.
// On Start, TUI lists task with a progress timer
func Full(out io.Writer, info io.Writer) api.EventProcessor {
	return &ttyWriter{
		out:   out,
		info:  info,
		tasks: map[string]task{},
		done:  make(chan bool),
		mtx:   &sync.Mutex{},
	}
}

type ttyWriter struct {
	out       io.Writer
	ids       []string // tasks ids ordered as first event appeared
	tasks     map[string]task
	repeated  bool
	numLines  int
	done      chan bool
	mtx       *sync.Mutex
	dryRun    bool // FIXME(ndeloof) (re)implement support for dry-run
	operation string
	ticker    *time.Ticker
	suspended bool
	info      io.Writer
}

type task struct {
	ID        string
	parent    string            // the resource this task receives updates from - other parents will be ignored
	parents   utils.Set[string] // all resources to depend on this task
	startTime time.Time
	endTime   time.Time
	text      string
	details   string
	status    api.EventStatus
	current   int64
	percent   int
	total     int64
	spinner   *Spinner
}

func newTask(e api.Resource) task {
	t := task{
		ID:        e.ID,
		parents:   utils.NewSet[string](),
		startTime: time.Now(),
		text:      e.Text,
		details:   e.Details,
		status:    e.Status,
		current:   e.Current,
		percent:   e.Percent,
		total:     e.Total,
		spinner:   NewSpinner(),
	}
	if e.ParentID != "" {
		t.parent = e.ParentID
		t.parents.Add(e.ParentID)
	}
	if e.Status == api.Done || e.Status == api.Error {
		t.stop()
	}
	return t
}

// update adjusts task state based on last received event
func (t *task) update(e api.Resource) {
	if e.ParentID != "" {
		t.parents.Add(e.ParentID)
		// we may receive same event from distinct parents (typically: images sharing layers)
		// to avoid status to flicker, only accept updates from our first declared parent
		if t.parent != e.ParentID {
			return
		}
	}

	// update task based on received event
	switch e.Status {
	case api.Done, api.Error, api.Warning:
		if t.status != e.Status {
			t.stop()
		}
	case api.Working:
		t.hasMore()
	}
	t.status = e.Status
	t.text = e.Text
	t.details = e.Details
	// progress can only go up
	if e.Total > t.total {
		t.total = e.Total
	}
	if e.Current > t.current {
		t.current = e.Current
	}
	if e.Percent > t.percent {
		t.percent = e.Percent
	}
}

func (t *task) stop() {
	t.endTime = time.Now()
	t.spinner.Stop()
}

func (t *task) hasMore() {
	t.spinner.Restart()
}

func (t *task) Completed() bool {
	switch t.status {
	case api.Done, api.Error, api.Warning:
		return true
	default:
		return false
	}
}

func (w *ttyWriter) Start(ctx context.Context, operation string) {
	w.ticker = time.NewTicker(100 * time.Millisecond)
	w.operation = operation
	go func() {
		for {
			select {
			case <-ctx.Done():
				// interrupted
				w.ticker.Stop()
				return
			case <-w.done:
				return
			case <-w.ticker.C:
				w.print()
			}
		}
	}()
}

func (w *ttyWriter) Done(operation string, success bool) {
	w.print()
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.ticker.Stop()
	w.operation = ""
	w.done <- true
}

func (w *ttyWriter) On(events ...api.Resource) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, e := range events {
		if e.ID == "Compose" {
			_, _ = fmt.Fprintln(w.info, ErrorColor(e.Details))
			continue
		}

		if w.operation != "start" && (e.Text == api.StatusStarted || e.Text == api.StatusStarting) {
			// skip those events to avoid mix with container logs
			continue
		}
		w.event(e)
	}
}

func (w *ttyWriter) event(e api.Resource) {
	// Suspend print while a build is in progress, to avoid collision with buildkit Display
	if e.Text == api.StatusBuilding {
		w.ticker.Stop()
		w.suspended = true
	} else if w.suspended {
		w.ticker.Reset(100 * time.Millisecond)
		w.suspended = false
	}

	if last, ok := w.tasks[e.ID]; ok {
		last.update(e)
		w.tasks[e.ID] = last
	} else {
		t := newTask(e)
		w.tasks[e.ID] = t
		w.ids = append(w.ids, e.ID)
	}
	w.printEvent(e)
}

func (w *ttyWriter) printEvent(e api.Resource) {
	if w.operation != "" {
		// event will be displayed by progress UI on ticker's ticks
		return
	}

	var color colorFunc
	switch e.Status {
	case api.Working:
		color = SuccessColor
	case api.Done:
		color = SuccessColor
	case api.Warning:
		color = WarningColor
	case api.Error:
		color = ErrorColor
	}
	_, _ = fmt.Fprintf(w.out, "%s %s %s\n", e.ID, color(e.Text), e.Details)
}

func (w *ttyWriter) parentTasks() iter.Seq[task] {
	return func(yield func(task) bool) {
		for _, id := range w.ids { // iterate on ids to enforce a consistent order
			t := w.tasks[id]
			if len(t.parents) == 0 {
				yield(t)
			}
		}
	}
}

func (w *ttyWriter) childrenTasks(parent string) iter.Seq[task] {
	return func(yield func(task) bool) {
		for _, id := range w.ids { // iterate on ids to enforce a consistent order
			t := w.tasks[id]
			if t.parents.Has(parent) {
				yield(t)
			}
		}
	}
}

func (w *ttyWriter) print() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if len(w.tasks) == 0 {
		return
	}
	terminalWidth := goterm.Width()
	up := w.numLines + 1
	if !w.repeated {
		up--
		w.repeated = true
	}
	b := aec.NewBuilder(
		aec.Hide, // Hide the cursor while we are printing
		aec.Up(uint(up)),
		aec.Column(0),
	)
	_, _ = fmt.Fprint(w.out, b.ANSI)
	defer func() {
		_, _ = fmt.Fprint(w.out, aec.Show)
	}()

	firstLine := fmt.Sprintf("[+] %s %d/%d", w.operation, numDone(w.tasks), len(w.tasks))
	_, _ = fmt.Fprintln(w.out, firstLine)

	var statusPadding int
	for _, t := range w.tasks {
		l := len(t.ID)
		if len(t.parents) == 0 && statusPadding < l {
			statusPadding = l
		}
	}

	skipChildEvents := len(w.tasks) > goterm.Height()-2
	numLines := 0
	for t := range w.parentTasks() {
		line := w.lineText(t, "", terminalWidth, statusPadding, w.dryRun)
		_, _ = fmt.Fprint(w.out, line)
		numLines++
		if skipChildEvents {
			continue
		}
		for child := range w.childrenTasks(t.ID) {
			line := w.lineText(child, "  ", terminalWidth, statusPadding-2, w.dryRun)
			_, _ = fmt.Fprint(w.out, line)
			numLines++
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
	if t.status != api.Working {
		endTime = t.startTime
		if (t.endTime != time.Time{}) {
			endTime = t.endTime
		}
	}
	prefix := ""
	if dryRun {
		prefix = PrefixColor(DRYRUN_PREFIX)
	}

	elapsed := endTime.Sub(t.startTime).Seconds()

	var (
		hideDetails bool
		total       int64
		current     int64
		completion  []string
	)

	// only show the aggregated progress while the root operation is in-progress
	if parent := t; parent.status == api.Working {
		for child := range w.childrenTasks(parent.ID) {
			if child.status == api.Working && child.total == 0 {
				// we don't have totals available for all the child events
				// so don't show the total progress yet
				hideDetails = true
			}
			total += child.total
			current += child.current
			completion = append(completion, percentChars[(len(percentChars)-1)*child.percent/100])
		}
	}

	// don't try to show detailed progress if we don't have any idea
	if total == 0 {
		hideDetails = true
	}

	txt := t.ID
	if len(completion) > 0 {
		var progress string
		if !hideDetails {
			progress = fmt.Sprintf(" %7s / %-7s", units.HumanSize(float64(current)), units.HumanSize(float64(total)))
		}
		txt = fmt.Sprintf("%s [%s]%s",
			t.ID,
			SuccessColor(strings.Join(completion, "")),
			progress,
		)
	}
	textLen := len(txt)
	padding := statusPadding - textLen
	if padding < 0 {
		padding = 0
	}
	// calculate the max length for the status text, on errors it
	// is 2-3 lines long and breaks the line formatting
	maxDetailsLen := terminalWidth - textLen - statusPadding - 15
	details := t.details
	// in some cases (debugging under VS Code), terminalWidth is set to zero by goterm.Width() ; ensuring we don't tweak strings with negative char index
	if maxDetailsLen > 0 && len(details) > maxDetailsLen {
		details = details[:maxDetailsLen] + "..."
	}
	text := fmt.Sprintf("%s %s%s %s %s%s %s",
		pad,
		spinner(t),
		prefix,
		txt,
		strings.Repeat(" ", padding),
		colorFn(t.status)(t.text),
		details,
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
	case api.Done:
		return SuccessColor(spinnerDone)
	case api.Warning:
		return WarningColor(spinnerWarning)
	case api.Error:
		return ErrorColor(spinnerError)
	default:
		return CountColor(t.spinner.String())
	}
}

func colorFn(s api.EventStatus) colorFunc {
	switch s {
	case api.Done:
		return SuccessColor
	case api.Warning:
		return WarningColor
	case api.Error:
		return ErrorColor
	default:
		return nocolor
	}
}

func numDone(tasks map[string]task) int {
	i := 0
	for _, t := range tasks {
		if t.status != api.Working {
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
