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

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gotest.tools/v3/assert"
)

// TestTestdataDirsAreOwned enforces the testdata convention: every top-level
// testdata/<Name>/ directory belongs to exactly one test function of this
// package, resolved by name. This is what keeps testdata from degrading into
// a catch-all fixtures directory: a renamed or removed test cannot leave its
// files behind, and files cannot be shared between tests.
func TestTestdataDirsAreOwned(t *testing.T) {
	declared := map[string]bool{}
	testFn := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	sources, err := filepath.Glob("*_test.go")
	assert.NilError(t, err)
	for _, src := range sources {
		data, err := os.ReadFile(src)
		assert.NilError(t, err)
		for _, m := range testFn.FindAllStringSubmatch(string(data), -1) {
			declared[m[1]] = true
		}
	}

	entries, err := os.ReadDir("testdata")
	assert.NilError(t, err)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		assert.Assert(t, declared[e.Name()],
			"testdata/%s is owned by no test of this package: name it after its test, or delete it", e.Name())
	}
}
