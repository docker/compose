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
	"runtime"
	"testing"

	"gotest.tools/v3/icmd"
)

func TestComposeMetrics(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("catch specific failure metrics", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/does-not-exist/compose.yaml", "build")
		expectedErr := "fixtures/does-not-exist/compose.yaml: no such file or directory"
		if runtime.GOOS == "windows" {
			expectedErr = "does-not-exist\\compose.yaml: The system cannot find the path specified"
		}
		res.Assert(t, icmd.Expected{ExitCode: 14, Err: expectedErr})
		res = c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/wrong-composefile/compose.yaml", "up", "-d")
		res.Assert(t, icmd.Expected{ExitCode: 15, Err: "services.simple Additional property wrongField is not allowed"})
		res = c.RunDockerComposeCmdNoCheck(t, "up")
		res.Assert(t, icmd.Expected{ExitCode: 14, Err: "no configuration file provided: not found"})
		res = c.RunDockerComposeCmdNoCheck(t, "up", "-f", "fixtures/wrong-composefile/compose.yaml", "--menu=false")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: "unknown shorthand flag: 'f' in -f"})
		res = c.RunDockerComposeCmdNoCheck(t, "up", "--file", "fixtures/wrong-composefile/compose.yaml", "--menu=false")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: "unknown flag: --file"})
		res = c.RunDockerComposeCmdNoCheck(t, "donw", "--file", "fixtures/wrong-composefile/compose.yaml")
		res.Assert(t, icmd.Expected{ExitCode: 16, Err: `unknown docker command: "compose donw"`})
		res = c.RunDockerComposeCmdNoCheck(t, "--file", "fixtures/wrong-composefile/build-error.yml", "build")
		res.Assert(t, icmd.Expected{ExitCode: 17, Err: `line 17: unknown instruction: WRONG`})
		res = c.RunDockerComposeCmdNoCheck(t, "--file", "fixtures/wrong-composefile/build-error.yml", "up")
		res.Assert(t, icmd.Expected{ExitCode: 17, Err: `line 17: unknown instruction: WRONG`})
		res = c.RunDockerComposeCmdNoCheck(t, "--file", "fixtures/wrong-composefile/unknown-image.yml", "pull")
		res.Assert(t, icmd.Expected{ExitCode: 18, Err: `pull access denied for unknownimage, repository does not exist or may require 'docker login'`})
		res = c.RunDockerComposeCmdNoCheck(t, "--file", "fixtures/wrong-composefile/unknown-image.yml", "up")
		res.Assert(t, icmd.Expected{ExitCode: 18, Err: `pull access denied for unknownimage, repository does not exist or may require 'docker login'`})
	})
}
