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
	"net/http"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeBuild(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("build named and unnamed images", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "build-test-nginx")
		c.RunDockerOrExitError(t, "rmi", "custom-nginx")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build")

		res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
		c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
		c.RunDockerCmd(t, "image", "inspect", "custom-nginx")
	})

	t.Run("build with build-arg", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "build-test-nginx")
		c.RunDockerOrExitError(t, "rmi", "custom-nginx")

		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build", "--build-arg", "FOO=BAR")

		res := c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
		res.Assert(t, icmd.Expected{Out: `"FOO": "BAR"`})
	})

	t.Run("build with build-arg set by env", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "build-test-nginx")
		c.RunDockerOrExitError(t, "rmi", "custom-nginx")

		icmd.RunCmd(c.NewDockerComposeCmd(t,
			"--project-directory",
			"fixtures/build-test",
			"build",
			"--build-arg",
			"FOO"),
			func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "FOO=BAR")
			})

		res := c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
		res.Assert(t, icmd.Expected{Out: `"FOO": "BAR"`})
	})

	t.Run("build with multiple build-args ", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "-f", "multi-args-multiargs")
		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/multi-args", "build")

		icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "DOCKER_BUILDKIT=0")
		})

		res := c.RunDockerCmd(t, "image", "inspect", "multi-args-multiargs")
		res.Assert(t, icmd.Expected{Out: `"RESULT": "SUCCESS"`})
	})

	t.Run("build failed with ssh default value", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test", "build", "--ssh", "")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "invalid empty ssh agent socket: make sure SSH_AUTH_SOCK is set",
		})

	})

	t.Run("build succeed with ssh from Compose file", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "build-test-ssh")

		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/ssh", "build")
		c.RunDockerCmd(t, "image", "inspect", "build-test-ssh")
	})

	t.Run("build succeed with ssh from CLI", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "build-test-ssh")

		c.RunDockerComposeCmd(t, "-f", "fixtures/build-test/ssh/compose-without-ssh.yaml", "--project-directory",
			"fixtures/build-test/ssh", "build", "--no-cache", "--ssh", "fake-ssh=./fixtures/build-test/ssh/fake_rsa")
		c.RunDockerCmd(t, "image", "inspect", "build-test-ssh")
	})

	t.Run("build failed with wrong ssh key id from CLI", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "build-test-ssh")

		res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/build-test/ssh/compose-without-ssh.yaml",
			"--project-directory", "fixtures/build-test/ssh", "build", "--no-cache", "--ssh",
			"wrong-ssh=./fixtures/build-test/ssh/fake_rsa")
		res.Assert(t, icmd.Expected{
			ExitCode: 17,
			Err:      "failed to solve: rpc error: code = Unknown desc = unset ssh forward key fake-ssh",
		})
	})

	t.Run("build succeed as part of up with ssh from Compose file", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "build-test-ssh")

		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/ssh", "up", "-d", "--build")
		t.Cleanup(func() {
			c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/ssh", "down")
		})
		c.RunDockerCmd(t, "image", "inspect", "build-test-ssh")
	})

	t.Run("build as part of up", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "build-test-nginx")
		c.RunDockerOrExitError(t, "rmi", "custom-nginx")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "up", "-d")
		t.Cleanup(func() {
			c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "down")
		})

		res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
		res.Assert(t, icmd.Expected{Out: "COPY static2 /usr/share/nginx/html"})

		output := HTTPGetWithRetry(t, "http://localhost:8070", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))

		c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
		c.RunDockerCmd(t, "image", "inspect", "custom-nginx")
	})

	t.Run("no rebuild when up again", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "up", "-d")

		assert.Assert(t, !strings.Contains(res.Stdout(), "COPY static"), res.Stdout())
	})

	t.Run("rebuild when up --build", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--workdir", "fixtures/build-test", "up", "-d", "--build")

		res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
		res.Assert(t, icmd.Expected{Out: "COPY static2 /usr/share/nginx/html"})
	})

	t.Run("cleanup build project", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "down")
		c.RunDockerCmd(t, "rmi", "build-test-nginx")
		c.RunDockerCmd(t, "rmi", "custom-nginx")
	})
}

func TestBuildSecrets(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("build with secrets", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "build-test-secret")

		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/secrets", "build")

		res := icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "SOME_SECRET=bar")
		})

		res.Assert(t, icmd.Success)
	})
}

func TestBuildTags(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("build with tags", func(t *testing.T) {

		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "build-test-tags")

		c.RunDockerComposeCmd(t, "--project-directory", "./fixtures/build-test/tags", "build", "--no-cache")

		res := c.RunDockerCmd(t, "image", "inspect", "build-test-tags")
		expectedOutput := `"RepoTags": [
            "docker/build-test-tags:1.0.0",
            "build-test-tags:latest",
            "other-image-name:v1.0.0"
        ],
`
		res.Assert(t, icmd.Expected{Out: expectedOutput})
	})
}

func TestBuildImageDependencies(t *testing.T) {
	doTest := func(t *testing.T, cli *CLI) {
		resetState := func() {
			cli.RunDockerComposeCmd(t, "down", "--rmi=all", "-t=0")
		}
		resetState()
		t.Cleanup(resetState)

		// the image should NOT exist now
		res := cli.RunDockerOrExitError(t, "image", "inspect", "build-dependencies-service")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "Error: No such image: build-dependencies-service",
		})

		res = cli.RunDockerComposeCmd(t, "build")
		t.Log(res.Combined())

		res = cli.RunDockerCmd(t,
			"image", "inspect", "--format={{ index .RepoTags 0 }}",
			"build-dependencies-service")
		res.Assert(t, icmd.Expected{Out: "build-dependencies-service:latest"})
	}

	t.Run("ClassicBuilder", func(t *testing.T) {
		cli := NewParallelCLI(t, WithEnv(
			"DOCKER_BUILDKIT=0",
			"COMPOSE_FILE=./fixtures/build-dependencies/compose.yaml",
		))
		doTest(t, cli)
	})

	t.Run("BuildKit", func(t *testing.T) {
		t.Skip("See https://github.com/docker/compose/issues/9232")
	})
}
