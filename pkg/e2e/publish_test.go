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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestPublishChecks(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-explicit-profiles"

	t.Run("publish error environment", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-environment.yml",
			"-p", projectName, "alpha", "publish", "test/test")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has environment variable(s) declared. To avoid leaking sensitive data,`})
	})

	t.Run("publish error env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has env_file declared. To avoid leaking sensitive data,`})
	})

	t.Run("publish success environment", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-environment.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish success env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})
}
