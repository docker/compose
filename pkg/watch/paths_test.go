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

package watch

import (
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGreatestExistingAncestor(t *testing.T) {
	f := NewTempDirFixture(t)

	p, err := greatestExistingAncestor(f.Path())
	assert.NilError(t, err)
	assert.Equal(t, f.Path(), p)

	p, err = greatestExistingAncestor(f.JoinPath("missing"))
	assert.NilError(t, err)
	assert.Equal(t, f.Path(), p)

	missingTopLevel := "/missingDir/a/b/c"
	if runtime.GOOS == "windows" {
		missingTopLevel = `C:\missingDir\a\b\c`
	}
	_, err = greatestExistingAncestor(missingTopLevel)
	assert.ErrorContains(t, err, "cannot watch root directory")
}
