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
	"fmt"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestLineText(t *testing.T) {
	now := time.Now()
	ev := Event{
		ID:         "id",
		Text:       "Text",
		Status:     Working,
		StatusText: "Status",
		endTime:    now,
		startTime:  now,
		spinner: &spinner{
			chars: []string{"."},
		},
	}

	lineWidth := len(fmt.Sprintf("%s %s", ev.ID, ev.Text))

	out := lineText(ev, "", 50, lineWidth, true)
	assert.Equal(t, out, "\x1b[37m . id Text Status                            0.0s\n\x1b[0m")

	out = lineText(ev, "", 50, lineWidth, false)
	assert.Equal(t, out, " . id Text Status                            0.0s\n")

	ev.Status = Done
	out = lineText(ev, "", 50, lineWidth, true)
	assert.Equal(t, out, "\x1b[34m . id Text Status                            0.0s\n\x1b[0m")

	ev.Status = Error
	out = lineText(ev, "", 50, lineWidth, true)
	assert.Equal(t, out, "\x1b[31m . id Text Status                            0.0s\n\x1b[0m")
}

func TestLineTextSingleEvent(t *testing.T) {
	now := time.Now()
	ev := Event{
		ID:         "id",
		Text:       "Text",
		Status:     Done,
		StatusText: "Status",
		startTime:  now,
		spinner: &spinner{
			chars: []string{"."},
		},
	}

	lineWidth := len(fmt.Sprintf("%s %s", ev.ID, ev.Text))

	out := lineText(ev, "", 50, lineWidth, true)
	assert.Equal(t, out, "\x1b[34m . id Text Status                            0.0s\n\x1b[0m")
}

func TestErrorEvent(t *testing.T) {
	w := &ttyWriter{
		events: map[string]Event{},
		mtx:    &sync.Mutex{},
	}
	e := Event{
		ID:         "id",
		Text:       "Text",
		Status:     Working,
		StatusText: "Working",
		startTime:  time.Now(),
		spinner: &spinner{
			chars: []string{"."},
		},
	}
	// Fire "Working" event and check end time isn't touched
	w.Event(e)
	event, ok := w.events[e.ID]
	assert.Assert(t, ok)
	assert.Assert(t, event.endTime.Equal(time.Time{}))

	// Fire "Error" event and check end time is set
	e.Status = Error
	w.Event(e)
	event, ok = w.events[e.ID]
	assert.Assert(t, ok)
	assert.Assert(t, event.endTime.After(time.Now().Add(-10*time.Second)))
}
