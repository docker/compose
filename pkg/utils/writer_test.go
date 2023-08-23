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

//nolint:errcheck
func TestSplitWriter(t *testing.T) {
	var lines []string
	w := GetWriter(func(line string) {
		lines = append(lines, line)
	})
	w.Write([]byte("h"))
	w.Write([]byte("e"))
	w.Write([]byte("l"))
	w.Write([]byte("l"))
	w.Write([]byte("o"))
	w.Write([]byte("\n"))
	w.Write([]byte("world!\n"))
	assert.DeepEqual(t, lines, []string{"hello", "world!"})

}
