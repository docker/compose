/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
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
	c, err := determineCurrentContext("", "")
	require.NoError(t, err)
	require.Equal(t, "default", c)

	// If context flag set, use that
	c, err = determineCurrentContext("other-context", "")
	require.NoError(t, err)
	require.Equal(t, "other-context", c)

	// If no context flag, use config
	c, err = determineCurrentContext("", d)
	require.NoError(t, err)
	require.Equal(t, "some-context", c)

	// Ensure context flag overrides config
	c, err = determineCurrentContext("other-context", d)
	require.NoError(t, err)
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
