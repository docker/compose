/*
   Copyright 2020 Docker, Inc.

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

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/api/cli/cmd"
	"github.com/docker/api/cli/cmd/context"
	"github.com/docker/api/cli/cmd/login"
	"github.com/docker/api/cli/cmd/run"
	"github.com/docker/api/config"
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
	c := determineCurrentContext("", "")
	assert.Equal(t, c, "default")

	// If context flag set, use that
	c = determineCurrentContext("other-context", "")
	assert.Equal(t, c, "other-context")

	// If no context flag, use config
	c = determineCurrentContext("", d)
	assert.Equal(t, c, "some-context")

	// Ensure context flag overrides config
	c = determineCurrentContext("other-context", d)
	assert.Equal(t, "other-context", c)
}

func TestCheckOwnCommand(t *testing.T) {
	assert.Assert(t, isOwnCommand(login.Command()))
	assert.Assert(t, isOwnCommand(context.Command()))
	assert.Assert(t, isOwnCommand(cmd.ServeCommand()))
	assert.Assert(t, !isOwnCommand(run.Command()))
	assert.Assert(t, !isOwnCommand(cmd.ExecCommand()))
	assert.Assert(t, !isOwnCommand(cmd.LogsCommand()))
	assert.Assert(t, !isOwnCommand(cmd.PsCommand()))
}
