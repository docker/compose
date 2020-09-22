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

package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

var sampleConfig = []byte(`{
	"otherField": "value",
	"currentContext": "local"
}`)

func testConfigDir(t *testing.T) string {
	d, _ := ioutil.TempDir("", "")
	t.Cleanup(func() {
		_ = os.RemoveAll(d)
	})
	return d
}

func writeSampleConfig(t *testing.T, d string) {
	err := ioutil.WriteFile(filepath.Join(d, ConfigFileName), sampleConfig, 0644)
	assert.NilError(t, err)
}

func TestLoadFile(t *testing.T) {
	d := testConfigDir(t)
	writeSampleConfig(t, d)
	f, err := LoadFile(d)
	assert.NilError(t, err)
	assert.Equal(t, f.CurrentContext, "local")
}

func TestOverWriteCurrentContext(t *testing.T) {
	d := testConfigDir(t)
	writeSampleConfig(t, d)
	f, err := LoadFile(d)
	assert.NilError(t, err)
	assert.Equal(t, f.CurrentContext, "local")

	err = WriteCurrentContext(d, "overwrite")
	assert.NilError(t, err)
	f, err = LoadFile(d)
	assert.NilError(t, err)
	assert.Equal(t, f.CurrentContext, "overwrite")

	m := map[string]interface{}{}
	err = loadFile(filepath.Join(d, ConfigFileName), &m)
	assert.NilError(t, err)
	assert.Equal(t, "overwrite", m["currentContext"])
	assert.Equal(t, "value", m["otherField"])
}

// TestWriteDefaultContextToEmptyConfig tests a specific case seen on the CI:
// panic when setting context to default with empty config file
func TestWriteDefaultContextToEmptyConfig(t *testing.T) {
	d := testConfigDir(t)
	err := WriteCurrentContext(d, "default")
	assert.NilError(t, err)
	c, err := ioutil.ReadFile(filepath.Join(d, ConfigFileName))
	assert.NilError(t, err)
	assert.Equal(t, string(c), "{}")
}
