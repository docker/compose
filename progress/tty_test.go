package progress

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	out := lineText(ev, 50, lineWidth, true)
	assert.Equal(t, "\x1b[37m . id Text Status                            0.0s\n\x1b[0m", out)

	out = lineText(ev, 50, lineWidth, false)
	assert.Equal(t, " . id Text Status                            0.0s\n", out)

	ev.Status = Done
	out = lineText(ev, 50, lineWidth, true)
	assert.Equal(t, "\x1b[34m . id Text Status                            0.0s\n\x1b[0m", out)

	ev.Status = Error
	out = lineText(ev, 50, lineWidth, true)
	assert.Equal(t, "\x1b[31m . id Text Status                            0.0s\n\x1b[0m", out)
}
