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
		args     []string
		expected string
	}{
		{
			name:     "with long flags",
			args:     []string{"--debug", "run"},
			expected: "run",
		},
		{
			name:     "with short flags",
			args:     []string{"-d", "run"},
			expected: "run",
		},
		{
			name:     "with flags with value",
			args:     []string{"--debug", "--str", "str-value", "run"},
			expected: "run",
		},
		{
			name:     "with --",
			args:     []string{"--debug", "--str", "str-value", "--", "run"},
			expected: "",
		},
		{
			name:     "without a command",
			args:     []string{"--debug", "--str", "str-value"},
			expected: "",
		},
		{
			name:     "with unknown short flag",
			args:     []string{"-f", "run"},
			expected: "",
		},
		{
			name:     "with unknown long flag",
			args:     []string{"--unknown-flag", "run"},
			expected: "",
		},
		{
			name:     "management command",
			args:     []string{"image", "ls"},
			expected: "image ls",
		},
		{
			name:     "management command with flag",
			args:     []string{"image", "--test", "ls"},
			expected: "image",
		},
		{
			name:     "management subcommand with flag",
			args:     []string{"image", "ls", "-q"},
			expected: "image ls",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := getCommand(testCase.args, root.PersistentFlags())
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestEcs(t *testing.T) {
	root := &cobra.Command{}
	root.PersistentFlags().BoolP("debug", "d", false, "debug")
	root.PersistentFlags().String("str", "str", "str")

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "compose up",
			args:     []string{"ecs", "compose", "-f", "test", "up"},
			expected: "ecs compose up",
		},
		{
			name:     "compose up",
			args:     []string{"ecs", "compose", "--file", "test", "up"},
			expected: "ecs compose up",
		},
		{
			name:     "compose up",
			args:     []string{"ecs", "compose", "--file", "test", "-n", "test", "up"},
			expected: "ecs compose up",
		},
		{
			name:     "compose up",
			args:     []string{"ecs", "compose", "--file", "test", "--project-name", "test", "up"},
			expected: "ecs compose up",
		},
		{
			name:     "compose up",
			args:     []string{"ecs", "compose", "up"},
			expected: "ecs compose up",
		},
		{
			name:     "compose down",
			args:     []string{"ecs", "compose", "-f", "test", "down"},
			expected: "ecs compose down",
		},
		{
			name:     "compose down",
			args:     []string{"ecs", "compose", "down"},
			expected: "ecs compose down",
		},
		{
			name:     "compose ps",
			args:     []string{"ecs", "compose", "-f", "test", "ps"},
			expected: "ecs compose ps",
		},
		{
			name:     "compose ps",
			args:     []string{"ecs", "compose", "ps"},
			expected: "ecs compose ps",
		},
		{
			name:     "compose logs",
			args:     []string{"ecs", "compose", "-f", "test", "logs"},
			expected: "ecs compose logs",
		},
		{
			name:     "setup",
			args:     []string{"ecs", "setup"},
			expected: "ecs setup",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := getCommand(testCase.args, root.PersistentFlags())
			assert.Equal(t, testCase.expected, result)
		})
	}
}
