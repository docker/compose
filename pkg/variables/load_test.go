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
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestLoadVarsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.yaml")
	body := []byte(`variables:
  APP_VERSION: "1.4.2"
  PORT: 8080
  ENABLED: true
`)
	assert.NilError(t, os.WriteFile(path, body, 0o644))

	entries, err := LoadVarsFile(path, SourceRootFile)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 3)
	got := map[string]string{}
	for _, e := range entries {
		got[e.Name] = e.Value
	}
	assert.Equal(t, got["APP_VERSION"], "1.4.2")
	assert.Equal(t, got["PORT"], "8080")
	assert.Equal(t, got["ENABLED"], "true")
}

func TestLoadVarsFilePreservesOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.yaml")
	body := []byte(`variables:
  Z: "1"
  A: "2"
  M: "3"
`)
	assert.NilError(t, os.WriteFile(path, body, 0o644))

	entries, err := LoadVarsFile(path, SourceRootFile)
	assert.NilError(t, err)
	names := []string{entries[0].Name, entries[1].Name, entries[2].Name}
	assert.DeepEqual(t, names, []string{"Z", "A", "M"})
}

func TestLoadVarsFileMissingTopLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.yaml")
	assert.NilError(t, os.WriteFile(path, []byte("services: {}\n"), 0o644))

	_, err := LoadVarsFile(path, SourceRootFile)
	assert.ErrorContains(t, err, "top-level `variables:` key missing")
}

func TestLoadVarsFileNullErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vars.yaml")
	body := []byte("variables:\n  FOO: ~\n")
	assert.NilError(t, os.WriteFile(path, body, 0o644))

	_, err := LoadVarsFile(path, SourceRootFile)
	assert.ErrorContains(t, err, "null value")
}

func TestParseCLIVars(t *testing.T) {
	entries, err := ParseCLIVars([]string{"FOO=bar", "EMPTY=", "WITH=eq=ual"})
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 3)
	assert.Equal(t, entries[0].Value, "bar")
	assert.Equal(t, entries[1].Value, "")
	assert.Equal(t, entries[2].Value, "eq=ual")
}

func TestParseCLIVarsRejectsMissingEquals(t *testing.T) {
	_, err := ParseCLIVars([]string{"FOO"})
	assert.ErrorContains(t, err, "KEY=VALUE")
}
