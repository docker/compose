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
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/buger/goterm"
	"github.com/docker/go-units"
	"github.com/morikuni/aec"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/utils"
)

// Full creates an EventProcessor that render advanced UI within a terminal.
// On Start, TUI lists task with a progress timer
func Full(out io.Writer, info io.Writer) api.EventProcessor {
	return &ttyWriter{
		out:   out,
		info:  info,
		tasks: map[string]*task{},
		done:  make(chan bool),
		mtx:   &sync.Mutex{},
	}
}

type ttyWriter struct {
	out       io.Writer
	ids       []string // tasks ids ordered as first event appeared
	tasks     map[string]*task
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
	} else {
		t := newTask(e)
		w.tasks[e.ID] = &t
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

func (w *ttyWriter) parentTasks() iter.Seq[*task] {
	return func(yield func(*task) bool) {
		for _, id := range w.ids { // iterate on ids to enforce a consistent order
			t := w.tasks[id]
			if len(t.parents) == 0 {
				yield(t)
			}
		}
	}
}

func (w *ttyWriter) childrenTasks(parent string) iter.Seq[*task] {
	return func(yield func(*task) bool) {
		for _, id := range w.ids { // iterate on ids to enforce a consistent order
			t := w.tasks[id]
			if t.parents.Has(parent) {
				yield(t)
			}
		}
	}
}

// lineData holds pre-computed formatting for a task line
type lineData struct {
	spinner     string // rendered spinner with color
	prefix      string // dry-run prefix if any
	taskID      string // possibly abbreviated
	progress    string // progress bar and size info
	status      string // rendered status with color
	details     string // possibly abbreviated
	timer       string // rendered timer with color
	statusPad   int    // padding before status to align
	timerPad    int    // padding before timer to align
	statusColor colorFunc
}

func (w *ttyWriter) print() {
	terminalWidth := goterm.Width()
	terminalHeight := goterm.Height()
	if terminalWidth <= 0 {
		terminalWidth = 80
	}
	if terminalHeight <= 0 {
		terminalHeight = 24
	}
	w.printWithDimensions(terminalWidth, terminalHeight)
}

func (w *ttyWriter) printWithDimensions(terminalWidth, terminalHeight int) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if len(w.tasks) == 0 {
		return
	}

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

	// Collect parent tasks in original order
	allTasks := slices.Collect(w.parentTasks())

	// Available lines: terminal height - 2 (header line + potential "more" line)
	maxLines := terminalHeight - 2
	if maxLines < 1 {
		maxLines = 1
	}

	showMore := len(allTasks) > maxLines
	tasksToShow := allTasks
	if showMore {
		tasksToShow = allTasks[:maxLines-1] // Reserve one line for "more" message
	}

	// collect line data and compute timerLen
	lines := make([]lineData, len(tasksToShow))
	var timerLen int
	for i, t := range tasksToShow {
		lines[i] = w.prepareLineData(t)
		if len(lines[i].timer) > timerLen {
			timerLen = len(lines[i].timer)
		}
	}

	// shorten details/taskID to fit terminal width
	w.adjustLineWidth(lines, timerLen, terminalWidth)

	// compute padding
	w.applyPadding(lines, terminalWidth, timerLen)

	// Render lines
	numLines := 0
	for _, l := range lines {
		_, _ = fmt.Fprint(w.out, lineText(l))
		numLines++
	}

	if showMore {
		moreCount := len(allTasks) - len(tasksToShow)
		moreText := fmt.Sprintf(" ... %d more", moreCount)
		pad := terminalWidth - len(moreText)
		if pad < 0 {
			pad = 0
		}
		_, _ = fmt.Fprintf(w.out, "%s%s\n", moreText, strings.Repeat(" ", pad))
		numLines++
	}

	// Clear any remaining lines from previous render
	for i := numLines; i < w.numLines; i++ {
		_, _ = fmt.Fprintln(w.out, strings.Repeat(" ", terminalWidth))
		numLines++
	}
	w.numLines = numLines
}

func (w *ttyWriter) applyPadding(lines []lineData, terminalWidth int, timerLen int) {
	var maxBeforeStatus int
	for i := range lines {
		l := &lines[i]
		// Width before statusPad: space(1) + spinner(1) + prefix + space(1) + taskID + progress
		beforeStatus := 3 + lenAnsi(l.prefix) + utf8.RuneCountInString(l.taskID) + lenAnsi(l.progress)
		if beforeStatus > maxBeforeStatus {
			maxBeforeStatus = beforeStatus
		}
	}

	for i, l := range lines {
		// Position before statusPad: space(1) + spinner(1) + prefix + space(1) + taskID + progress
		beforeStatus := 3 + lenAnsi(l.prefix) + utf8.RuneCountInString(l.taskID) + lenAnsi(l.progress)
		// statusPad aligns status; lineText adds 1 more space after statusPad
		l.statusPad = maxBeforeStatus - beforeStatus

		// Format: beforeStatus + statusPad + space(1) + status
		lineLen := beforeStatus + l.statusPad + 1 + utf8.RuneCountInString(l.status)
		if l.details != "" {
			lineLen += 1 + utf8.RuneCountInString(l.details)
		}
		l.timerPad = terminalWidth - lineLen - timerLen
		if l.timerPad < 1 {
			l.timerPad = 1
		}
		lines[i] = l

	}
}

func (w *ttyWriter) adjustLineWidth(lines []lineData, timerLen int, terminalWidth int) {
	const minIDLen = 10
	maxStatusLen := maxStatusLength(lines)

	// Iteratively truncate until all lines fit
	for range 100 { // safety limit
		maxBeforeStatus := maxBeforeStatusWidth(lines)
		overflow := computeOverflow(lines, maxBeforeStatus, maxStatusLen, timerLen, terminalWidth)

		if overflow <= 0 {
			break
		}

		// First try to truncate details, then taskID
		if !truncateDetails(lines, overflow) && !truncateLongestTaskID(lines, overflow, minIDLen) {
			break // Can't truncate further
		}
	}
}

// maxStatusLength returns the maximum status text length across all lines.
func maxStatusLength(lines []lineData) int {
	var maxLen int
	for i := range lines {
		if len(lines[i].status) > maxLen {
			maxLen = len(lines[i].status)
		}
	}
	return maxLen
}

// maxBeforeStatusWidth computes the maximum width before statusPad across all lines.
// This is: space(1) + spinner(1) + prefix + space(1) + taskID + progress
func maxBeforeStatusWidth(lines []lineData) int {
	var maxWidth int
	for i := range lines {
		l := &lines[i]
		width := 3 + lenAnsi(l.prefix) + len(l.taskID) + lenAnsi(l.progress)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// computeOverflow calculates how many characters the widest line exceeds the terminal width.
// Returns 0 or negative if all lines fit.
func computeOverflow(lines []lineData, maxBeforeStatus, maxStatusLen, timerLen, terminalWidth int) int {
	var maxOverflow int
	for i := range lines {
		l := &lines[i]
		detailsLen := len(l.details)
		if detailsLen > 0 {
			detailsLen++ // space before details
		}
		// Line width: maxBeforeStatus + space(1) + status + details + minTimerPad(1) + timer
		lineWidth := maxBeforeStatus + 1 + maxStatusLen + detailsLen + 1 + timerLen
		overflow := lineWidth - terminalWidth
		if overflow > maxOverflow {
			maxOverflow = overflow
		}
	}
	return maxOverflow
}

// truncateDetails tries to truncate the first line's details to reduce overflow.
// Returns true if any truncation was performed.
func truncateDetails(lines []lineData, overflow int) bool {
	for i := range lines {
		l := &lines[i]
		if len(l.details) > 3 {
			reduction := overflow
			if reduction > len(l.details)-3 {
				reduction = len(l.details) - 3
			}
			l.details = l.details[:len(l.details)-reduction-3] + "..."
			return true
		} else if l.details != "" {
			l.details = ""
			return true
		}
	}
	return false
}

// truncateLongestTaskID truncates the longest taskID to reduce overflow.
// Returns true if truncation was performed.
func truncateLongestTaskID(lines []lineData, overflow, minIDLen int) bool {
	longestIdx := -1
	longestLen := minIDLen
	for i := range lines {
		if len(lines[i].taskID) > longestLen {
			longestLen = len(lines[i].taskID)
			longestIdx = i
		}
	}

	if longestIdx < 0 {
		return false
	}

	l := &lines[longestIdx]
	reduction := overflow + 3 // account for "..."
	newLen := len(l.taskID) - reduction
	if newLen < minIDLen-3 {
		newLen = minIDLen - 3
	}
	if newLen > 0 {
		l.taskID = l.taskID[:newLen] + "..."
	}
	return true
}

func (w *ttyWriter) prepareLineData(t *task) lineData {
	endTime := time.Now()
	if t.status != api.Working {
		endTime = t.startTime
		if (t.endTime != time.Time{}) {
			endTime = t.endTime
		}
	}

	prefix := ""
	if w.dryRun {
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
	if t.status == api.Working {
		for child := range w.childrenTasks(t.ID) {
			if child.status == api.Working && child.total == 0 {
				hideDetails = true
			}
			total += child.total
			current += child.current
			r := len(percentChars) - 1
			p := child.percent
			if p > 100 {
				p = 100
			}
			completion = append(completion, percentChars[r*p/100])
		}
	}

	if total == 0 {
		hideDetails = true
	}

	var progress string
	if len(completion) > 0 {
		progress = " [" + SuccessColor(strings.Join(completion, "")) + "]"
		if !hideDetails {
			progress += fmt.Sprintf(" %7s / %-7s", units.HumanSize(float64(current)), units.HumanSize(float64(total)))
		}
	}

	return lineData{
		spinner:     spinner(t),
		prefix:      prefix,
		taskID:      t.ID,
		progress:    progress,
		status:      t.text,
		statusColor: colorFn(t.status),
		details:     t.details,
		timer:       fmt.Sprintf("%.1fs", elapsed),
	}
}

func lineText(l lineData) string {
	var sb strings.Builder
	sb.WriteString(" ")
	sb.WriteString(l.spinner)
	sb.WriteString(l.prefix)
	sb.WriteString(" ")
	sb.WriteString(l.taskID)
	sb.WriteString(l.progress)
	sb.WriteString(strings.Repeat(" ", l.statusPad))
	sb.WriteString(" ")
	sb.WriteString(l.statusColor(l.status))
	if l.details != "" {
		sb.WriteString(" ")
		sb.WriteString(l.details)
	}
	sb.WriteString(strings.Repeat(" ", l.timerPad))
	sb.WriteString(TimerColor(l.timer))
	sb.WriteString("\n")
	return sb.String()
}

var (
	spinnerDone    = "✔"
	spinnerWarning = "!"
	spinnerError   = "✘"
)

func spinner(t *task) string {
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

func numDone(tasks map[string]*task) int {
	i := 0
	for _, t := range tasks {
		if t.status != api.Working {
			i++
		}
	}
	return i
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
