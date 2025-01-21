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
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has environment variable(s) declared.
To avoid leaking sensitive data,`})
	})

	t.Run("publish error env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has env_file declared.
service "serviceA" has environment variable(s) declared.
To avoid leaking sensitive data,`})
	})

	t.Run("publish multiple errors env_file and environment", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-multi-env-config.yml",
			"-p", projectName, "alpha", "publish", "test/test")
		// we don't in which order the services will be loaded, so we can't predict the order of the error messages
		assert.Assert(t, strings.Contains(res.Combined(), `service "serviceB" has env_file declared.`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `service "serviceB" has environment variable(s) declared.`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `service "serviceA" has environment variable(s) declared.`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `To avoid leaking sensitive data, you must either explicitly allow the sending of environment variables by using the --with-env flag,
or remove sensitive data from your Compose configuration
`), res.Combined())
	})

	t.Run("publish success environment", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-environment.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "-y", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish success env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "-y", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish approve validation message", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("y\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), "Are you ok to publish these environment variables? [y/N]:"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish refuse validation message", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), "Are you ok to publish these environment variables? [y/N]:"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish list env variables", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-multi-env-config.yml",
			"-p", projectName, "alpha", "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), `you are about to publish environment variables within your OCI artifact.
please double check that you are not leaking sensitive data`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `Service/Config  serviceA
FOO=bar`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `Service/Config  serviceB`), res.Combined())
		// we don't know in which order the env variables will be loaded
		assert.Assert(t, strings.Contains(res.Combined(), `FOO=bar`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `BAR=baz`), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), `QUIX=`), res.Combined())
	})
}
