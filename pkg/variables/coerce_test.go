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

package variables

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestCoerce(t *testing.T) {
	cases := []struct {
		name    string
		in      any
		want    string
		wantErr string
	}{
		{name: "string", in: "hello", want: "hello"},
		{name: "int", in: 8080, want: "8080"},
		{name: "int64", in: int64(8080), want: "8080"},
		{name: "true", in: true, want: "true"},
		{name: "false", in: false, want: "false"},
		{name: "float", in: 3.14, want: "3.14"},
		{name: "null", in: nil, wantErr: "null value"},
		{name: "list", in: []any{"a"}, wantErr: "unsupported value type"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Coerce(c.name, c.in)
			if c.wantErr != "" {
				assert.ErrorContains(t, err, c.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, c.want)
		})
	}
}
