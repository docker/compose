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

package transform

import (
	"reflect"
	"testing"

	"gotest.tools/v3/assert"
)

func TestReplace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple",
			in: `services:
  test:
    extends:
      file: foo.yaml
      service: foo
`,
			want: `services:
  test:
    extends:
      file: REPLACED
      service: foo
`,
		},
		{
			name: "last line",
			in: `services:
  test:
    extends:
      service: foo
      file: foo.yaml
`,
			want: `services:
  test:
    extends:
      service: foo
      file: REPLACED
`,
		},
		{
			name: "last line no CR",
			in: `services:
  test:
    extends:
      service: foo
      file: foo.yaml`,
			want: `services:
  test:
    extends:
      service: foo
      file: REPLACED`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceExtendsFile([]byte(tt.in), "test", "REPLACED")
			assert.NilError(t, err)
			if !reflect.DeepEqual(got, []byte(tt.want)) {
				t.Errorf("ReplaceExtendsFile() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// A flow-style document puts every node on line 1: the position scan in
// replace never sees a newline preceding the target, pos runs to the end of
// the stream, and the column offset then slices out of range. compose
// publish feeds the user's original file bytes into this path
// (processFile), so the flow style is reachable input, not a hypothetical.
func TestReplaceFlowStyle(t *testing.T) {
	t.Run("extends file", func(t *testing.T) {
		_, err := ReplaceExtendsFile([]byte(`{services: {test: {extends: {file: old.yaml}}}}`), "test", "REPLACED")
		assert.ErrorContains(t, err, "flow-style")
	})
	t.Run("env_file", func(t *testing.T) {
		_, err := ReplaceEnvFile([]byte(`{services: {test: {env_file: old.env}}}`), "test", 0, "REPLACED")
		assert.ErrorContains(t, err, "flow style")
	})
	// on main this silently corrupted the stream: rewriting an entry of a
	// flow sequence swallowed the rest of the line, closing bracket included
	t.Run("env_file flow sequence", func(t *testing.T) {
		_, err := ReplaceEnvFile([]byte(`{services: {test: {env_file: [a.env, b.env]}}}`), "test", 1, "REPLACED")
		assert.ErrorContains(t, err, "flow style")
	})
	t.Run("env_file out-of-range entry", func(t *testing.T) {
		_, err := ReplaceEnvFile([]byte(`services:
  test:
    env_file:
      - a.env
      - b.env
`), "test", 5, "REPLACED")
		assert.ErrorContains(t, err, "cannot rewrite entry 5")
	})
}
