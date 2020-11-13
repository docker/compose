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

package metrics

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestGetCommand(t *testing.T) {
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
			args:     []string{"-D", "run"},
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
			name:     "management command",
			args:     []string{"image", "ls"},
			expected: "image ls",
		},
		{
			name:     "management command with flag",
			args:     []string{"image", "--test", "ls"},
			expected: "image ls",
		},
		{
			name:     "management subcommand with flag",
			args:     []string{"image", "ls", "-q"},
			expected: "image ls",
		},
		{
			name:     "azure login",
			args:     []string{"login", "azure"},
			expected: "login azure",
		},
		{
			name:     "azure logout",
			args:     []string{"logout", "azure"},
			expected: "logout azure",
		},
		{
			name:     "azure login with flags",
			args:     []string{"login", "-u", "test", "azure"},
			expected: "login azure",
		},
		{
			name:     "login to a registry",
			args:     []string{"login", "myregistry"},
			expected: "login",
		},
		{
			name:     "logout from a registry",
			args:     []string{"logout", "myregistry"},
			expected: "logout",
		},
		{
			name:     "context create aci",
			args:     []string{"context", "create", "aci"},
			expected: "context create aci",
		},
		{
			name:     "context create ecs",
			args:     []string{"context", "create", "ecs"},
			expected: "context create ecs",
		},
		{
			name:     "create a context from another context",
			args:     []string{"context", "create", "test-context", "--from=default"},
			expected: "context create",
		},
		{
			name:     "create a container",
			args:     []string{"create"},
			expected: "create",
		},
		{
			name:     "start a container named aci",
			args:     []string{"start", "aci"},
			expected: "start",
		},
		{
			name:     "create a container named test-container",
			args:     []string{"create", "test-container"},
			expected: "create",
		},
		{
			name:     "create with flags",
			args:     []string{"create", "--rm", "test"},
			expected: "create",
		},
		{
			name:     "compose up -f xxx",
			args:     []string{"compose", "up", "-f", "titi.yaml"},
			expected: "compose up",
		},
		{
			name:     "compose -f xxx up",
			args:     []string{"compose", "-f", "titi.yaml", "up"},
			expected: "compose up",
		},
		{
			name:     "-D compose -f xxx up",
			args:     []string{"--debug", "compose", "-f", "titi.yaml", "up"},
			expected: "compose up",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := GetCommand(testCase.args)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestKeepHelpCommands(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "run with help flag",
			args:     []string{"run", "--help"},
			expected: "--help run",
		},
		{
			name:     "with help flag before-after commands",
			args:     []string{"compose", "--help", "up"},
			expected: "--help compose up",
		},
		{
			name:     "help flag",
			args:     []string{"--help"},
			expected: "--help",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := GetCommand(testCase.args)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestEcs(t *testing.T) {
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
			name:     "ecs",
			args:     []string{"ecs", "anything"},
			expected: "ecs",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := GetCommand(testCase.args)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestScan(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "scan",
			args:     []string{"scan"},
			expected: "scan",
		},
		{
			name:     "scan image with long flags",
			args:     []string{"scan", "--file", "file", "myimage"},
			expected: "scan",
		},
		{
			name:     "scan image with short flags",
			args:     []string{"scan", "-f", "file", "myimage"},
			expected: "scan",
		},
		{
			name:     "scan with long flag",
			args:     []string{"scan", "--dependency-tree", "myimage"},
			expected: "scan",
		},
		{
			name:     "auth",
			args:     []string{"scan", "--login"},
			expected: "scan --login",
		},
		{
			name:     "version",
			args:     []string{"scan", "--version"},
			expected: "scan --version",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := GetCommand(testCase.args)
			assert.Equal(t, testCase.expected, result)
		})
	}
}
