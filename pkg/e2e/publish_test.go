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
			"-p", projectName, "publish", "test/test")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has environment variable(s) declared.
To avoid leaking sensitive data,`})
	})

	t.Run("publish error env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "publish", "test/test")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `service "serviceA" has env_file declared.
service "serviceA" has environment variable(s) declared.
To avoid leaking sensitive data,`})
	})

	t.Run("publish multiple errors env_file and environment", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-multi-env-config.yml",
			"-p", projectName, "publish", "test/test")
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
			"-p", projectName, "publish", "test/test", "--with-env", "-y", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish success env_file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "-y", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish approve validation message", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("y\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), "Are you ok to publish these environment variables?"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish refuse validation message", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-env-file.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), "Are you ok to publish these environment variables?"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "test/test publishing"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish with extends", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/compose-with-extends.yml",
			"-p", projectName, "publish", "test/test", "--dry-run")
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("publish list env variables", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-multi-env-config.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		out := res.Combined()
		assert.Assert(t, strings.Contains(out, `you are about to publish environment variables within your OCI artifact.
please double check that you are not leaking sensitive data`), out)
		assert.Assert(t, strings.Contains(out, `Service/Config  serviceA
FOO=bar`), out)
		assert.Assert(t, strings.Contains(out, `Service/Config  serviceB`), out)
		// we don't know in which order the env variables will be loaded
		assert.Assert(t, strings.Contains(out, `FOO=bar`), out)
		assert.Assert(t, strings.Contains(out, `BAR=baz`), out)
		assert.Assert(t, strings.Contains(out, `QUIX=`), out)
	})

	t.Run("refuse to publish with bind mount", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-bind-mount.yml",
			"-p", projectName, "publish", "test/test", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		out := res.Combined()
		assert.Assert(t, strings.Contains(out, "you are about to publish bind mounts declaration within your OCI artifact."), out)
		assert.Assert(t, strings.Contains(out, "e2e/fixtures/publish:/user-data"), out)
		assert.Assert(t, strings.Contains(out, "Are you ok to publish these bind mount declarations?"), out)
		assert.Assert(t, !strings.Contains(out, "serviceA published"), out)
	})

	t.Run("publish with bind mount", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-bind-mount.yml",
			"-p", projectName, "publish", "test/test", "--dry-run")
		cmd.Stdin = strings.NewReader("y\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})
		assert.Assert(t, strings.Contains(res.Combined(), "you are about to publish bind mounts declaration within your OCI artifact."), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Are you ok to publish these bind mount declarations?"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e/fixtures/publish:/user-data"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "test/test published"), res.Combined())
	})

	t.Run("refuse to publish with build section only", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-build-only.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "-y", "--dry-run")
		res.Assert(t, icmd.Expected{ExitCode: 1})
		assert.Assert(t, strings.Contains(res.Combined(), "your Compose stack cannot be published as it only contains a build section for service(s):"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "serviceA"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "serviceB"), res.Combined())
	})

	t.Run("refuse to publish with local include", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/publish/compose-local-include.yml",
			"-p", projectName, "publish", "test/test", "--dry-run")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: "cannot publish compose file with local includes"})
	})

	t.Run("detect sensitive data", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/publish/compose-sensitive.yml",
			"-p", projectName, "publish", "test/test", "--with-env", "--dry-run")
		cmd.Stdin = strings.NewReader("n\n")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{ExitCode: 0})

		output := res.Combined()
		assert.Assert(t, strings.Contains(output, "you are about to publish sensitive data within your OCI artifact.\n"), output)
		assert.Assert(t, strings.Contains(output, "please double check that you are not leaking sensitive data"), output)
		assert.Assert(t, strings.Contains(output, "AWS Client ID\n\"services.serviceA.environment.AWS_ACCESS_KEY_ID\": A3TX1234567890ABCDEF"), output)
		assert.Assert(t, strings.Contains(output, "AWS Secret Key\n\"services.serviceA.environment.AWS_SECRET_ACCESS_KEY\": aws\"12345+67890/abcdefghijklm+NOPQRSTUVWXYZ+\""), output)
		assert.Assert(t, strings.Contains(output, "Github authentication\n\"GITHUB_TOKEN\": ghp_1234567890abcdefghijklmnopqrstuvwxyz"), output)
		assert.Assert(t, strings.Contains(output, "JSON Web Token\n\"\": eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9."+
			"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw"), output)
		assert.Assert(t, strings.Contains(output, "Private Key\n\"\": -----BEGIN DSA PRIVATE KEY-----\nwxyz+ABC=\n-----END DSA PRIVATE KEY-----"), output)
	})
}

func TestPublish(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-publish"
	const registryName = projectName + "-registry"
	c.RunDockerCmd(t, "run", "--name", registryName, "-P", "-d", "registry:3")
	port := c.RunDockerCmd(t, "inspect", "--format", `{{ (index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort }}`, registryName).Stdout()
	registry := "localhost:" + strings.TrimSpace(port)
	t.Cleanup(func() {
		c.RunDockerCmd(t, "rm", "--force", registryName)
	})

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/publish/oci/compose.yaml", "-f", "./fixtures/publish/oci/compose-override.yaml",
		"-p", projectName, "publish", "--with-env", "--yes", "--insecure-registry", registry+"/test:test")
	res.Assert(t, icmd.Expected{ExitCode: 0})

	// docker exec -it compose-e2e-publish-registry tree /var/lib/registry/docker/registry/v2/

	cmd := c.NewDockerComposeCmd(t, "--verbose", "--project-name=oci",
		"--insecure-registry", registry,
		"-f", fmt.Sprintf("oci://%s/test:test", registry), "config")
	res = icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
		cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+t.TempDir())
	})
	res.Assert(t, icmd.Expected{ExitCode: 0})
	assert.Equal(t, res.Stdout(), `name: oci
services:
  app:
    environment:
      HELLO: WORLD
    image: alpine
    networks:
      default: null
networks:
  default:
    name: oci_default
`)
}
