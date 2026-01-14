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

package paths

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestExpandUser(t *testing.T) {
	home, err := os.UserHomeDir()
	assert.NilError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "tilde with slash",
			input:    "~/.env",
			expected: filepath.Join(home, ".env"),
		},
		{
			name:     "tilde with subdir",
			input:    "~/subdir/.env",
			expected: filepath.Join(home, "subdir", ".env"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/absolute/path/.env",
			expected: "/absolute/path/.env",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path/.env",
			expected: "relative/path/.env",
		},
		{
			name:     "tilde in middle unchanged",
			input:    "/path/~/file",
			expected: "/path/~/file",
		},
		{
			name:     "tilde other user unchanged",
			input:    "~otheruser/.env",
			expected: "~otheruser/.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandUser(tt.input)
			assert.Equal(t, result, tt.expected)
		})
	}
}
