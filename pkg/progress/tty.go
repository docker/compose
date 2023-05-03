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
	out           io.Writer
	events        map[string]Event
	eventIDs      []string
	repeated      bool
	numLines      int
	done          chan bool
	mtx           *sync.Mutex
	tailEvents    []string
	dryRun        bool
	progressTitle string
}

func (w *ttyWriter) children(id string) []Event {
	var events []Event
	for _, v := range w.eventIDs {
		ev := w.events[v]
		if ev.ParentID == id {
			events = append(events, ev)
		}
	}
	return events
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
		}
		last.Status = e.Status
		last.Text = e.Text
		last.StatusText = e.StatusText
		last.Total = e.Total
		last.Current = e.Current
		last.Percent = e.Percent
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
	for _, e := range events {
		w.Event(e)
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
		fmt.Fprintln(w.out, msg)
	}
}

const showLogs = 5

func (w *ttyWriter) print() {
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
	fmt.Fprint(w.out, b.Column(0).ANSI)

	// Hide the cursor while we are printing
	fmt.Fprint(w.out, aec.Hide)
	defer fmt.Fprint(w.out, aec.Show)

	topLevelEvents := w.topLevelEvents()
	done := countDone(topLevelEvents)
	firstLine := fmt.Sprintf("[+] %s %d/%d", w.progressTitle, done, len(topLevelEvents))
	if done == len(topLevelEvents) {
		firstLine = DoneColor(firstLine)
	}
	fmt.Fprintln(w.out, firstLine)

	height := goterm.Height() - 2
	numLines := 0
	height = height - len(topLevelEvents)
	statusPadding := computePadding(topLevelEvents)
	for _, event := range topLevelEvents {
		line := w.lineText(event, true, "", terminalWidth, statusPadding, w.dryRun)
		fmt.Fprint(w.out, line)
		numLines++

		if event.Status == Working {
			children := w.children(event.ID)
			if len(children) <= height {
				height = height - len(children)
				for _, ev := range children {
					line := w.lineText(ev, false, "  ", terminalWidth, 0, w.dryRun)
					fmt.Fprint(w.out, line)
					numLines++

					if ev.Status == Working && height >= showLogs {
						ch := w.children(ev.ID)
						if len(ch) > showLogs {
							w.dropEvents(ch[:len(ch)-showLogs])
							ch = ch[len(ch)-showLogs:]
						}
						for _, ev := range ch {
							line := w.lineText(ev, false, "     ", terminalWidth, 0, w.dryRun)
							fmt.Fprint(w.out, line)
							numLines++
						}
					}
				}
			}

		}
	}
	for i := numLines; i < w.numLines; i++ {
		if numLines < goterm.Height()-2 {
			fmt.Fprintln(w.out, strings.Repeat(" ", terminalWidth))
			numLines++
		}
	}
	w.numLines = numLines
}

func (w *ttyWriter) topLevelEvents() []Event {
	var events []Event
	for _, id := range w.eventIDs {
		event := w.events[id]
		if event.ParentID == "" {
			events = append(events, event)
		}
	}
	return events
}

func computePadding(events []Event) int {
	var statusPadding int
	for _, event := range events {
		l := len(fmt.Sprintf("%s %s", event.ID, event.Text))
		if statusPadding < l {
			statusPadding = l
		}
	}
	return statusPadding
}

func countDone(events []Event) int {
	var count int
	for _, event := range events {
		if event.Status == Done {
			count++
		}
	}
	return count
}

func (w *ttyWriter) lineText(event Event, showID bool, pad string, terminalWidth, statusPadding int, dryRun bool) string {
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
		total      int64
		current    int64
		completion []string
	)

	for _, v := range w.eventIDs {
		ev := w.events[v]
		if ev.ParentID == event.ID {
			total += ev.Total
			current += ev.Current
			if ev.Percent != 0 {
				completion = append(completion, percentChars[(len(percentChars)-1)*ev.Percent/100])
			}
		}
	}

	var txt string
	if showID {
		prefix = fmt.Sprintf("%s %s", prefix, event.ID)
	}
	if len(completion) > 0 {
		txt = fmt.Sprintf("%s %s%s%s [%s] %7s/%-7s %s",
			pad,
			event.Spinner(),
			prefix,
			CountColor(fmt.Sprintf("%d layers", len(completion))),
			SuccessColor(strings.Join(completion, "")),
			units.HumanSize(float64(current)), units.HumanSize(float64(total)),
			event.Text)
	} else {
		txt = fmt.Sprintf("%s %s%s %s",
			pad,
			event.Spinner(),
			prefix,
			event.Text)
	}
	textLen := len(txt)
	padding := statusPadding - textLen
	if padding < 0 {
		padding = 0
	}
	// calculate the max length for the status text, on errors it
	// is 2-3 lines long and breaks the line formatting
	maxStatusLen := terminalWidth - textLen - statusPadding
	status := event.StatusText
	// in some cases (debugging under VS Code), terminalWidth is set to zero by goterm.Width() ; ensuring we don't tweak strings with negative char index
	if maxStatusLen > 0 && len(status) > maxStatusLen {
		status = status[:maxStatusLen] + "..."
	}
	text := fmt.Sprintf("%s%s %s",
		txt,
		strings.Repeat(" ", padding),
		event.Status.colorFn()(status),
	)
	timer := fmt.Sprintf("%.1fs ", elapsed)
	o := align(text, TimerColor(timer), terminalWidth)

	return o
}

func (w *ttyWriter) dropEvents(events []Event) {
	for _, e := range events {
		delete(w.events, e.ID)
	}
	ids := make([]string, 0, len(w.eventIDs)-len(events))
	for _, id := range w.eventIDs {
		skip := false
		for _, e := range events {
			if id == e.ID {
				skip = true
				break
			}
		}
		if !skip {
			ids = append(ids, id)
		}
	}
	w.eventIDs = ids
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
