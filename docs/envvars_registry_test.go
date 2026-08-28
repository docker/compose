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

package docs

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

var composeVarLiteral = regexp.MustCompile(`"(COMPOSE_[A-Z_0-9]+)"`)

// Every COMPOSE_* variable named in production code must have a row in
// docs/envvars.md — the registry is only useful while it is exhaustive, and
// nothing but this test keeps it so. e2e sources are excluded: they set
// variables to exercise them, they don't define new ones.
func TestEnvVarRegistryIsExhaustive(t *testing.T) {
	registry, err := os.ReadFile("envvars.md")
	assert.NilError(t, err)

	inCode := map[string][]string{}
	for _, root := range []string{"../cmd", "../pkg", "../internal"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "e2e" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range composeVarLiteral.FindAllStringSubmatch(string(src), -1) {
				name := m[1]
				if name == "COMPOSE_" { // prefix checks, not variables
					continue
				}
				inCode[name] = append(inCode[name], path)
			}
			return nil
		})
		assert.NilError(t, err)
	}
	assert.Assert(t, len(inCode) > 10, "the sweep found suspiciously few variables — did the source layout move?")

	for name, sites := range inCode {
		assert.Assert(t, strings.Contains(string(registry), "`"+name+"`"),
			"%s is read in code (%s) but has no row in docs/envvars.md — add it (or remove the dead read)", name, strings.Join(sites, ", "))
	}
}
