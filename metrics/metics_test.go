/*
   Copyright 2020 Docker, Inc.

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

package metrics

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestFlag(t *testing.T) {
	root := &cobra.Command{}
	root.PersistentFlags().BoolP("debug", "d", false, "debug")
	root.PersistentFlags().String("str", "str", "str")

	testCases := []struct {
		name     string
		flags    []string
		expected string
	}{
		{
			name:     "with long flags",
			flags:    []string{"--debug", "run"},
			expected: "run",
		},
		{
			name:     "with short flags",
			flags:    []string{"-d", "run"},
			expected: "run",
		},
		{
			name:     "with flags with value",
			flags:    []string{"--debug", "--str", "str-value", "run"},
			expected: "run",
		},
		{
			name:     "with --",
			flags:    []string{"--debug", "--str", "str-value", "--", "run"},
			expected: "",
		},
		{
			name:     "without a command",
			flags:    []string{"--debug", "--str", "str-value"},
			expected: "",
		},
		{
			name:     "with unknown short flag",
			flags:    []string{"-f", "run"},
			expected: "",
		},
		{
			name:     "with unknown long flag",
			flags:    []string{"--unknown-flag", "run"},
			expected: "",
		},
		{
			name:     "management command",
			flags:    []string{"image", "ls"},
			expected: "image ls",
		},
		{
			name:     "management command with flag",
			flags:    []string{"image", "--test", "ls"},
			expected: "image",
		},
		{
			name:     "management subcommand with flag",
			flags:    []string{"image", "ls", "-q"},
			expected: "image ls",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := getCommand(testCase.flags, root.PersistentFlags())
			assert.Equal(t, testCase.expected, result)
		})
	}
}
