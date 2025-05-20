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
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestLogConsumer_Log(t *testing.T) {
	type testStruct struct {
		name    string
		message string
	}

	container := "app"
	red := "\033[31m"
	reset := "\033[0m"
	testList := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:    "single line",
			message: "This is a first line",
			expected: fmt.Sprintf(
				"%s  | %s\n",
				container, "This is a first line",
			),
		},
		{
			name: "multiple lines",
			message: fmt.Sprintf(
				"%s\n%s",
				"This is a first line",
				"This is a second line",
			),
			expected: fmt.Sprintf(
				"%s  | %s\n%s  | %s\n",
				container, "This is a first line",
				container, "This is a second line",
			),
		},
		{
			name:    "single line (colored)",
			message: fmt.Sprint("This is ", red, "RED", reset),
			expected: fmt.Sprintf(
				"%s  | %s%s%s%s\n",
				container, "This is ", red, "RED", reset,
			),
		},
		{
			name: "multiple lines (colored)",
			message: fmt.Sprintf(
				"%s%s%s\n%s\n%s%s%s",
				"This is ", red, "RED",
				"This line is also RED",
				"This is ", reset, "BLACK",
			),
			expected: fmt.Sprintf(
				"%s  | %s%s%s\n%s  | %s%s\n%s  | %s%s%s%s\n",
				container, "This is ", red, "RED",
				// inherit the previous line color
				container, red, "This line is also RED",
				container, red, "This is ", reset, "BLACK",
			),
		},
	}

	color, prefix, timestamp := false, true, false
	for _, test := range testList {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.TODO() // TODO: use t.Context() in go1.24
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			logConsumer := NewLogConsumer(ctx, stdout, stderr, color, prefix, timestamp)
			logConsumer.Register(container)
			logConsumer.Log(container, test.message)
			assert.Equal(t, stdout.String(), test.expected)
		})
	}
}
