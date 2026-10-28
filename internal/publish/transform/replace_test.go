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
