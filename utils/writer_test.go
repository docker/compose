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

package utils

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestSplitWriter(t *testing.T) {
	var lines []string
	w := GetWriter(func(line string) {
		lines = append(lines, line)
	})
	w.Write([]byte("h"))        //nolint: errcheck
	w.Write([]byte("e"))        //nolint: errcheck
	w.Write([]byte("l"))        //nolint: errcheck
	w.Write([]byte("l"))        //nolint: errcheck
	w.Write([]byte("o"))        //nolint: errcheck
	w.Write([]byte("\n"))       //nolint: errcheck
	w.Write([]byte("world!\n")) //nolint: errcheck
	assert.DeepEqual(t, lines, []string{"hello", "world!"})

}
