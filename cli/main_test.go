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

	"github.com/docker/api/cli/cmd"
	"github.com/docker/api/cli/cmd/context"
	"github.com/docker/api/cli/cmd/login"
	"github.com/docker/api/cli/cmd/run"

	"github.com/stretchr/testify/require"

	"github.com/docker/api/config"
)

var contextSetConfig = []byte(`{
	"currentContext": "some-context"
}`)

func TestDetermineCurrentContext(t *testing.T) {
	d, err := ioutil.TempDir("", "")
	// nolint errcheck
	defer os.RemoveAll(d)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(d, config.ConfigFileName), contextSetConfig, 0644)
	require.NoError(t, err)

	// If nothing set, fallback to default
	c := determineCurrentContext("", "")
	require.Equal(t, "default", c)

	// If context flag set, use that
	c = determineCurrentContext("other-context", "")
	require.Equal(t, "other-context", c)

	// If no context flag, use config
	c = determineCurrentContext("", d)
	require.Equal(t, "some-context", c)

	// Ensure context flag overrides config
	c = determineCurrentContext("other-context", d)
	require.Equal(t, "other-context", c)
}

func TestCheckOwnCommand(t *testing.T) {
	require.True(t, isOwnCommand(login.Command()))
	require.True(t, isOwnCommand(context.Command()))
	require.True(t, isOwnCommand(cmd.ServeCommand()))
	require.False(t, isOwnCommand(run.Command()))
	require.False(t, isOwnCommand(cmd.ExecCommand()))
	require.False(t, isOwnCommand(cmd.LogsCommand()))
	require.False(t, isOwnCommand(cmd.PsCommand()))
}
