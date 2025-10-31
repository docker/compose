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

package formatter

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestANSIStatePreservation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "red color across multiple lines",
			input: "\033[31mThis line is RED.\nThis line is also RED.\033[0m",
			expected: []string{
				"This line is RED.",
				"This line is also RED.",
			},
		},
		{
			name:  "color change within multiline",
			input: "\033[31mThis is RED.\nStill RED.\nNow \033[34mBLUE.\033[0m",
			expected: []string{
				"This is RED.",
				"Still RED.",
				"Now \033[34mBLUE.",
			},
		},
		{
			name:  "no ANSI codes",
			input: "Plain text\nMore plain text",
			expected: []string{
				"Plain text",
				"More plain text",
			},
		},
		{
			name:  "single line with ANSI",
			input: "\033[32mGreen text\033[0m",
			expected: []string{
				"Green text",
			},
		},
		{
			name:  "reset in middle of multiline",
			input: "\033[31mRed\nStill red\033[0m\nNow normal\nStill normal",
			expected: []string{
				"Red",
				"Still red",
				"Now normal",
				"Still normal",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			consumer := NewLogConsumer(context.Background(), buf, buf, false, false, false)
			consumer.Log("test", tt.input)

			output := buf.String()
			lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

			assert.Equal(t, len(tt.expected), len(lines), "number of lines should match")

			for i, expectedContent := range tt.expected {
				lineWithoutANSI := stripANSIExceptContent(lines[i])
				assert.Assert(t, strings.Contains(lineWithoutANSI, expectedContent),
					"line %d should contain expected content. got: %q, want to contain: %q",
					i, lineWithoutANSI, expectedContent)
			}
		})
	}
}

func TestExtractANSIState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "red color code",
			input:    "\033[31mRed text",
			expected: "\033[31m",
		},
		{
			name:     "reset code",
			input:    "\033[31mRed\033[0m",
			expected: "",
		},
		{
			name:     "no ANSI codes",
			input:    "Plain text",
			expected: "",
		},
		{
			name:     "multiple codes",
			input:    "\033[1m\033[31mBold red",
			expected: "\033[1;31m",
		},
		{
			name:     "code then reset",
			input:    "\033[31mRed\033[0mNormal",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractANSIState(tt.input)
			if tt.expected == "" {
				assert.Equal(t, "", result)
			} else {
				assert.Assert(t, result != "", "expected non-empty ANSI state")
			}
		})
	}
}

func TestHasANSICodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "with ANSI codes",
			input:    "\033[31mRed text\033[0m",
			expected: true,
		},
		{
			name:     "no ANSI codes",
			input:    "Plain text",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasANSICodes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func stripANSIExceptContent(s string) string {
	return strings.TrimSpace(s)
}
