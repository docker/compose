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
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func newTestWriter() (*ttyWriter, *bytes.Buffer) {
	var buf bytes.Buffer
	w := &ttyWriter{
		out:       &buf,
		info:      &buf,
		tasks:     map[string]*task{},
		done:      make(chan bool),
		mtx:       &sync.Mutex{},
		operation: "pull",
	}
	return w, &buf
}

func addTask(w *ttyWriter, id, text, details string, status api.EventStatus) {
	t := &task{
		ID:        id,
		parents:   make(map[string]struct{}),
		startTime: time.Now(),
		text:      text,
		details:   details,
		status:    status,
		spinner:   NewSpinner(),
	}
	w.tasks[id] = t
	w.ids = append(w.ids, id)
}

// extractLines parses the output buffer and returns lines without ANSI control sequences
func extractLines(buf *bytes.Buffer) []string {
	content := buf.String()
	// Split by newline
	rawLines := strings.Split(content, "\n")
	var lines []string
	for _, line := range rawLines {
		// Skip empty lines and lines that are just ANSI codes
		if lenAnsi(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestPrintWithDimensions_LinesFitTerminalWidth(t *testing.T) {
	testCases := []struct {
		name          string
		taskID        string
		status        string
		details       string
		terminalWidth int
	}{
		{
			name:          "short task fits wide terminal",
			taskID:        "Image foo",
			status:        "Pulling",
			details:       "layer abc123",
			terminalWidth: 100,
		},
		{
			name:          "long details truncated to fit",
			taskID:        "Image foo",
			status:        "Pulling",
			details:       "downloading layer sha256:abc123def456789xyz0123456789abcdef",
			terminalWidth: 50,
		},
		{
			name:          "long taskID truncated to fit",
			taskID:        "very-long-image-name-that-exceeds-terminal-width",
			status:        "Pulling",
			details:       "",
			terminalWidth: 40,
		},
		{
			name:          "both long taskID and details",
			taskID:        "my-very-long-service-name-here",
			status:        "Downloading",
			details:       "layer sha256:abc123def456789xyz0123456789",
			terminalWidth: 50,
		},
		{
			name:          "narrow terminal",
			taskID:        "service-name",
			status:        "Pulling",
			details:       "some details",
			terminalWidth: 35,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w, buf := newTestWriter()
			addTask(w, tc.taskID, tc.status, tc.details, api.Working)

			w.printWithDimensions(tc.terminalWidth, 24)

			lines := extractLines(buf)
			for i, line := range lines {
				lineLen := lenAnsi(line)
				assert.Assert(t, lineLen <= tc.terminalWidth,
					"line %d has length %d which exceeds terminal width %d: %q",
					i, lineLen, tc.terminalWidth, line)
			}
		})
	}
}

func TestPrintWithDimensions_MultipleTasksFitTerminalWidth(t *testing.T) {
	w, buf := newTestWriter()

	// Add multiple tasks with varying lengths
	addTask(w, "Image nginx", "Pulling", "layer sha256:abc123", api.Working)
	addTask(w, "Image postgres-database", "Pulling", "downloading", api.Working)
	addTask(w, "Image redis", "Pulled", "", api.Done)

	terminalWidth := 60
	w.printWithDimensions(terminalWidth, 24)

	lines := extractLines(buf)
	for i, line := range lines {
		lineLen := lenAnsi(line)
		assert.Assert(t, lineLen <= terminalWidth,
			"line %d has length %d which exceeds terminal width %d: %q",
			i, lineLen, terminalWidth, line)
	}
}

func TestPrintWithDimensions_VeryNarrowTerminal(t *testing.T) {
	w, buf := newTestWriter()
	addTask(w, "Image nginx", "Pulling", "details", api.Working)

	terminalWidth := 30
	w.printWithDimensions(terminalWidth, 24)

	lines := extractLines(buf)
	for i, line := range lines {
		lineLen := lenAnsi(line)
		assert.Assert(t, lineLen <= terminalWidth,
			"line %d has length %d which exceeds terminal width %d: %q",
			i, lineLen, terminalWidth, line)
	}
}

func TestPrintWithDimensions_TaskWithProgress(t *testing.T) {
	w, buf := newTestWriter()

	// Create parent task
	parent := &task{
		ID:        "Image nginx",
		parents:   make(map[string]struct{}),
		startTime: time.Now(),
		text:      "Pulling",
		status:    api.Working,
		spinner:   NewSpinner(),
	}
	w.tasks["Image nginx"] = parent
	w.ids = append(w.ids, "Image nginx")

	// Create child tasks to trigger progress display
	for i := 0; i < 3; i++ {
		child := &task{
			ID:        "layer" + string(rune('a'+i)),
			parents:   map[string]struct{}{"Image nginx": {}},
			startTime: time.Now(),
			text:      "Downloading",
			status:    api.Working,
			total:     1000,
			current:   500,
			percent:   50,
			spinner:   NewSpinner(),
		}
		w.tasks[child.ID] = child
		w.ids = append(w.ids, child.ID)
	}

	terminalWidth := 80
	w.printWithDimensions(terminalWidth, 24)

	lines := extractLines(buf)
	for i, line := range lines {
		lineLen := lenAnsi(line)
		assert.Assert(t, lineLen <= terminalWidth,
			"line %d has length %d which exceeds terminal width %d: %q",
			i, lineLen, terminalWidth, line)
	}
}

func TestAdjustLineWidth_DetailsCorrectlyTruncated(t *testing.T) {
	w := &ttyWriter{}
	lines := []lineData{
		{
			taskID:  "Image foo",
			status:  "Pulling",
			details: "downloading layer sha256:abc123def456789xyz",
		},
	}

	terminalWidth := 50
	timerLen := 5
	w.adjustLineWidth(lines, timerLen, terminalWidth)

	// Verify the line fits
	detailsLen := len(lines[0].details)
	if detailsLen > 0 {
		detailsLen++ // space before details
	}
	// widthWithoutDetails = 5 + prefix(0) + taskID(9) + progress(0) + status(7) + timer(5) = 26
	lineWidth := 5 + len(lines[0].taskID) + len(lines[0].status) + detailsLen + timerLen

	assert.Assert(t, lineWidth <= terminalWidth,
		"line width %d should not exceed terminal width %d (taskID=%q, details=%q)",
		lineWidth, terminalWidth, lines[0].taskID, lines[0].details)

	// Verify details were truncated (not removed entirely)
	assert.Assert(t, lines[0].details != "", "details should be truncated, not removed")
	assert.Assert(t, strings.HasSuffix(lines[0].details, "..."), "truncated details should end with ...")
}

func TestAdjustLineWidth_TaskIDCorrectlyTruncated(t *testing.T) {
	w := &ttyWriter{}
	lines := []lineData{
		{
			taskID:  "very-long-image-name-that-exceeds-minimum-length",
			status:  "Pulling",
			details: "",
		},
	}

	terminalWidth := 40
	timerLen := 5
	w.adjustLineWidth(lines, timerLen, terminalWidth)

	lineWidth := 5 + len(lines[0].taskID) + 7 + timerLen

	assert.Assert(t, lineWidth <= terminalWidth,
		"line width %d should not exceed terminal width %d (taskID=%q)",
		lineWidth, terminalWidth, lines[0].taskID)

	assert.Assert(t, strings.HasSuffix(lines[0].taskID, "..."), "truncated taskID should end with ...")
}

func TestAdjustLineWidth_NoTruncationNeeded(t *testing.T) {
	w := &ttyWriter{}
	originalDetails := "short"
	originalTaskID := "Image foo"
	lines := []lineData{
		{
			taskID:  originalTaskID,
			status:  "Pulling",
			details: originalDetails,
		},
	}

	// Wide terminal, nothing should be truncated
	w.adjustLineWidth(lines, 5, 100)

	assert.Equal(t, originalTaskID, lines[0].taskID, "taskID should not be modified")
	assert.Equal(t, originalDetails, lines[0].details, "details should not be modified")
}

func TestAdjustLineWidth_DetailsRemovedWhenTooShort(t *testing.T) {
	w := &ttyWriter{}
	lines := []lineData{
		{
			taskID:  "Image foo",
			status:  "Pulling",
			details: "abc", // Very short, can't be meaningfully truncated
		},
	}

	// Terminal so narrow that even minimal details + "..." wouldn't help
	w.adjustLineWidth(lines, 5, 28)

	assert.Equal(t, "", lines[0].details, "details should be removed entirely when too short to truncate")
}

// stripAnsi removes ANSI escape codes from a string
func stripAnsi(s string) string {
	var result strings.Builder
	inAnsi := false
	for _, r := range s {
		if r == '\x1b' {
			inAnsi = true
			continue
		}
		if inAnsi {
			// ANSI sequences end with a letter (m, h, l, G, etc.)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inAnsi = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func TestPrintWithDimensions_PulledAndPullingWithLongIDs(t *testing.T) {
	w, buf := newTestWriter()

	// Add a completed task with long ID
	completedTask := &task{
		ID:        "Image docker.io/library/nginx-long-name",
		parents:   make(map[string]struct{}),
		startTime: time.Now().Add(-2 * time.Second),
		endTime:   time.Now(),
		text:      "Pulled",
		status:    api.Done,
		spinner:   NewSpinner(),
	}
	completedTask.spinner.Stop()
	w.tasks[completedTask.ID] = completedTask
	w.ids = append(w.ids, completedTask.ID)

	// Add a pending task with long ID
	pendingTask := &task{
		ID:        "Image docker.io/library/postgres-database",
		parents:   make(map[string]struct{}),
		startTime: time.Now(),
		text:      "Pulling",
		status:    api.Working,
		spinner:   NewSpinner(),
	}
	w.tasks[pendingTask.ID] = pendingTask
	w.ids = append(w.ids, pendingTask.ID)

	terminalWidth := 50
	w.printWithDimensions(terminalWidth, 24)

	// Strip all ANSI codes from output and split by newline
	stripped := stripAnsi(buf.String())
	lines := strings.Split(stripped, "\n")

	// Filter non-empty lines
	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	// Expected output format (50 runes per task line)
	expected := `[+] pull 1/2
 ✔ Image docker.io/library/nginx-l... Pulled  2.0s
 ⠋ Image docker.io/library/postgre... Pulling 0.0s`

	expectedLines := strings.Split(expected, "\n")

	// Debug output
	t.Logf("Actual output:\n")
	for i, line := range nonEmptyLines {
		t.Logf("  line %d (%2d runes): %q", i, utf8.RuneCountInString(line), line)
	}

	// Verify number of lines
	assert.Equal(t, len(expectedLines), len(nonEmptyLines), "number of lines should match")

	// Verify each line matches expected
	for i, line := range nonEmptyLines {
		if i < len(expectedLines) {
			assert.Equal(t, expectedLines[i], line,
				"line %d should match expected", i)
		}
	}

	// Verify task lines fit within terminal width (strict - no tolerance)
	for i, line := range nonEmptyLines {
		if i > 0 { // Skip header line
			runeCount := utf8.RuneCountInString(line)
			assert.Assert(t, runeCount <= terminalWidth,
				"line %d has %d runes which exceeds terminal width %d: %q",
				i, runeCount, terminalWidth, line)
		}
	}
}

func TestLenAnsi(t *testing.T) {
	testCases := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"\x1b[32mhello\x1b[0m", 5},
		{"\x1b[1;32mgreen\x1b[0m text", 10},
		{"", 0},
		{"\x1b[0m", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := lenAnsi(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
