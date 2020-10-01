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

package run

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"

	"github.com/docker/compose-cli/cli/options/run"
)

func TestHelp(t *testing.T) {
	var b bytes.Buffer
	c := Command("aci")
	c.SetOutput(&b)
	_ = c.Help()
	golden.Assert(t, b.String(), "run-help.golden")
}

func TestHelpNoDomainFlag(t *testing.T) {
	var b bytes.Buffer
	c := Command("default")
	c.SetOutput(&b)
	_ = c.Help()
	assert.Assert(t, !strings.Contains(b.String(), "domainname"))
}

func TestRunEnvironmentFiles(t *testing.T) {
	runOpts := run.Opts{
		Environment: []string{
			"VAR=1",
		},
		EnvironmentFiles: []string{
			"testdata/runtest1.env",
			"testdata/runtest2.env",
		},
	}
	containerConfig, err := runOpts.ToContainerConfig("test")
	assert.NilError(t, err)
	assert.DeepEqual(t, containerConfig.Environment, []string{
		"VAR=1",
		"FIRST_VAR=\"firstValue\"",
		"SECOND_VAR=secondValue",
		"THIRD_VAR=2",
		"FOURTH_VAR=fourthValue",
	})
}
