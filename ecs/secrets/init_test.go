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

package secrets

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/fs"
)

func TestRawSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	err := os.Setenv("raw", "something_secret")
	assert.NilError(t, err)
	defer os.Unsetenv("raw") // nolint:errcheck

	err = CreateSecretFiles(Secret{
		Name: "raw",
		Keys: nil,
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "raw"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "something_secret")
}

func TestSelectedKeysSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	err := os.Setenv("json", `
{
   "foo": "bar",
   "zot": "qix"
}`)
	assert.NilError(t, err)
	defer os.Unsetenv("json") // nolint:errcheck

	err = CreateSecretFiles(Secret{
		Name: "json",
		Keys: []string{"foo"},
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "json", "foo"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "bar")

	_, err = os.Stat(filepath.Join(dir, "json", "zot"))
	assert.Check(t, os.IsNotExist(err))
}

func TestAllKeysSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()
	err := os.Setenv("json", `
{
   "foo": "bar",
   "zot": "qix"
}`)
	assert.NilError(t, err)
	defer os.Unsetenv("json") // nolint:errcheck

	err = CreateSecretFiles(Secret{
		Name: "json",
		Keys: []string{"*"},
	}, dir)
	assert.NilError(t, err)
	file, err := ioutil.ReadFile(filepath.Join(dir, "json", "foo"))
	assert.NilError(t, err)
	content := string(file)
	assert.Equal(t, content, "bar")

	file, err = ioutil.ReadFile(filepath.Join(dir, "json", "zot"))
	assert.NilError(t, err)
	content = string(file)
	assert.Equal(t, content, "qix")
}

func TestUnknownSecret(t *testing.T) {
	dir := fs.NewDir(t, "secrets").Path()

	err := CreateSecretFiles(Secret{
		Name: "not_set",
		Keys: nil,
	}, dir)
	assert.Check(t, err != nil)
}
