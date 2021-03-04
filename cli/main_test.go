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

package main

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose-cli/cli/cmd"
	"github.com/docker/compose-cli/cli/cmd/context"
	"github.com/docker/compose-cli/cli/cmd/login"
	"github.com/docker/compose-cli/cli/cmd/run"
)

func TestCheckOwnCommand(t *testing.T) {
	assert.Assert(t, isContextAgnosticCommand(login.Command()))
	assert.Assert(t, isContextAgnosticCommand(context.Command()))
	assert.Assert(t, isContextAgnosticCommand(cmd.ServeCommand()))
	assert.Assert(t, !isContextAgnosticCommand(run.Command("default")))
	assert.Assert(t, !isContextAgnosticCommand(cmd.ExecCommand()))
	assert.Assert(t, !isContextAgnosticCommand(cmd.LogsCommand()))
	assert.Assert(t, !isContextAgnosticCommand(cmd.PsCommand()))
}

func TestAppendPaths(t *testing.T) {
	assert.Equal(t, appendPaths("", "/bin/path"), "/bin/path")
	assert.Equal(t, appendPaths("path1", "binaryPath"), "path1"+string(os.PathListSeparator)+"binaryPath")
}
