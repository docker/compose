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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGreatestExistingAncestor(t *testing.T) {
	f := NewTempDirFixture(t)

	p, err := greatestExistingAncestor(f.Path())
	require.NoError(t, err)
	assert.Equal(t, f.Path(), p)

	p, err = greatestExistingAncestor(f.JoinPath("missing"))
	require.NoError(t, err)
	assert.Equal(t, f.Path(), p)

	missingTopLevel := "/missingDir/a/b/c"
	if runtime.GOOS == "windows" {
		missingTopLevel = "C:\\missingDir\\a\\b\\c"
	}
	_, err = greatestExistingAncestor(missingTopLevel)
	assert.Contains(t, err.Error(), "cannot watch root directory")
}
