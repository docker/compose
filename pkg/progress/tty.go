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
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/docker/compose/v2/pkg/utils"

	"github.com/buger/goterm"
	"github.com/morikuni/aec"
)

type ttyWriter struct {
	out        io.Writer
	events     map[string]Event
	eventIDs   []string
	repeated   bool
	numLines   int
	done       chan bool
	mtx        *sync.Mutex
	tailEvents []string
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
		case Done, Error:
			if last.Status != e.Status {
				last.stop()
			}
		}
		last.Status = e.Status
		last.Text = e.Text
		last.StatusText = e.StatusText
		last.ParentID = e.ParentID
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

func (w *ttyWriter) TailMsgf(msg string, args ...interface{}) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	w.tailEvents = append(w.tailEvents, fmt.Sprintf(msg, args...))
}

func (w *ttyWriter) printTailEvents() {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	for _, msg := range w.tailEvents {
		fmt.Fprintln(w.out, msg)
	}
}

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

	firstLine := fmt.Sprintf("[+] Running %d/%d", numDone(w.events), w.numLines)
	if w.numLines != 0 && numDone(w.events) == w.numLines {
		firstLine = aec.Apply(firstLine, aec.BlueF)
	}
	fmt.Fprintln(w.out, firstLine)

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

	numLines := 0
	for _, v := range w.eventIDs {
		event := w.events[v]
		if event.ParentID != "" {
			continue
		}
		line := lineText(event, "", terminalWidth, statusPadding, runtime.GOOS != "windows")
		// nolint: errcheck
		fmt.Fprint(w.out, line)
		numLines++
		for _, v := range w.eventIDs {
			ev := w.events[v]
			if ev.ParentID == event.ID {
				line := lineText(ev, "  ", terminalWidth, statusPadding, runtime.GOOS != "windows")
				// nolint: errcheck
				fmt.Fprint(w.out, line)
				numLines++
			}
		}
	}

	w.numLines = numLines
}

func lineText(event Event, pad string, terminalWidth, statusPadding int, color bool) string {
	endTime := time.Now()
	if event.Status != Working {
		endTime = event.startTime
		if (event.endTime != time.Time{}) {
			endTime = event.endTime
		}
	}

	elapsed := endTime.Sub(event.startTime).Seconds()

	textLen := len(fmt.Sprintf("%s %s", event.ID, event.Text))
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
	text := fmt.Sprintf("%s %s %s %s%s %s",
		pad,
		event.spinner.String(),
		event.ID,
		event.Text,
		strings.Repeat(" ", padding),
		status,
	)
	timer := fmt.Sprintf("%.1fs\n", elapsed)
	o := align(text, timer, terminalWidth)

	if color {
		color := aec.WhiteF
		if event.Status == Done {
			color = aec.BlueF
		}
		if event.Status == Error {
			color = aec.RedF
		}
		return aec.Apply(o, color)
	}

	return o
}

func numDone(events map[string]Event) int {
	i := 0
	for _, e := range events {
		if e.Status == Done {
			i++
		}
	}
	return i
}

func align(l, r string, w int) string {
	return fmt.Sprintf("%-[2]*[1]s %[3]s", l, w-len(r)-1, r)
}
