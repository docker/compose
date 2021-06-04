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

package e2e

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/utils/e2e"
)

func TestComposeMetrics(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	s := NewMetricsServer(c.MetricsSocket())
	s.Start()
	defer s.Stop()

	started := false
	for i := 0; i < 30; i++ {
		c.RunDockerCmd("help", "ps")
		if len(s.GetUsage()) > 0 {
			started = true
			fmt.Printf("	[%s] Server up in %d ms\n", t.Name(), i*100)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.Assert(t, started, "Metrics mock server not available after 3 secs")

	t.Run("catch specific failure metrics", func(t *testing.T) {
		s.ResetUsage()

		res := c.RunDockerOrExitError("compose", "-f", "../compose/fixtures/does-not-exist/compose.yml", "build")
		expectedErr := "compose/fixtures/does-not-exist/compose.yml: no such file or directory"
		if runtime.GOOS == "windows" {
			expectedErr = "does-not-exist\\compose.yml: The system cannot find the path specified"
		}
		res.Assert(t, icmd.Expected{ExitCode: 14, Err: expectedErr})
		res = c.RunDockerOrExitError("compose", "-f", "../compose/fixtures/wrong-composefile/compose.yml", "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 15, Err: "services.simple Additional property wrongField is not allowed"})
		res = c.RunDockerOrExitError("compose", "up")
		res.Assert(t, icmd.Expected{ExitCode: 14, Err: "can't find a suitable configuration file in this directory or any parent: not found"})
		res = c.RunDockerOrExitError("compose", "up", "-f", "../compose/fixtures/wrong-composefile/compose.yml")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: "unknown shorthand flag: 'f' in -f"})
		res = c.RunDockerOrExitError("compose", "up", "--file", "../compose/fixtures/wrong-composefile/compose.yml")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: "unknown flag: --file"})
		res = c.RunDockerOrExitError("compose", "donw", "--file", "../compose/fixtures/wrong-composefile/compose.yml")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: `unknown docker command: "compose donw"`})
		res = c.RunDockerOrExitError("compose", "--file", "../compose/fixtures/wrong-composefile/build-error.yml", "build")
		res.Assert(t, icmd.Expected{ExitCode: 17, Err: `line 17: unknown instruction: WRONG`})
		res = c.RunDockerOrExitError("compose", "--file", "../compose/fixtures/wrong-composefile/build-error.yml", "up")
		res.Assert(t, icmd.Expected{ExitCode: 17, Err: `line 17: unknown instruction: WRONG`})
		res = c.RunDockerOrExitError("compose", "--file", "../compose/fixtures/wrong-composefile/unknown-image.yml", "pull")
		res.Assert(t, icmd.Expected{ExitCode: 18, Err: `pull access denied for unknownimage, repository does not exist or may require 'docker login'`})
		res = c.RunDockerOrExitError("compose", "--file", "../compose/fixtures/wrong-composefile/unknown-image.yml", "up")
		res.Assert(t, icmd.Expected{ExitCode: 18, Err: `pull access denied for unknownimage, repository does not exist or may require 'docker login'`})

		usage := s.GetUsage()
		assert.DeepEqual(t, []string{
			`{"command":"compose build","context":"moby","source":"cli","status":"failure-file-not-found"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-compose-parse"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-file-not-found"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-cmd-syntax"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-cmd-syntax"}`,
			`{"command":"compose","context":"moby","source":"cli","status":"failure-cmd-syntax"}`,
			`{"command":"compose build","context":"moby","source":"cli","status":"failure-build"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-build"}`,
			`{"command":"compose pull","context":"moby","source":"cli","status":"failure-pull"}`,
			`{"command":"compose up","context":"moby","source":"cli","status":"failure-pull"}`,
		}, usage)
	})
}
