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

	"github.com/docker/compose-cli/api/config"
)

var contextSetConfig = []byte(`{
	"currentContext": "some-context"
}`)

func TestDetermineCurrentContext(t *testing.T) {
	d, err := ioutil.TempDir("", "")
	// nolint errcheck
	defer os.RemoveAll(d)
	assert.NilError(t, err)
	err = ioutil.WriteFile(filepath.Join(d, config.ConfigFileName), contextSetConfig, 0644)
	assert.NilError(t, err)

	// If nothing set, fallback to default
	c := GetCurrentContext("", "", []string{})
	assert.Equal(t, c, "default")

	// If context flag set, use that
	c = GetCurrentContext("other-context", "", []string{})
	assert.Equal(t, c, "other-context")

	// If no context flag, use config
	c = GetCurrentContext("", d, []string{})
	assert.Equal(t, c, "some-context")

	// Ensure context flag overrides config
	c = GetCurrentContext("other-context", d, []string{})
	assert.Equal(t, "other-context", c)

	// Ensure host flag overrides context
	c = GetCurrentContext("other-context", d, []string{"hostname"})
	assert.Equal(t, "default", c)
}
