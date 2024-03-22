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
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeBuild(t *testing.T) {

	for _, env := range []string{"DOCKER_BUILDKIT=0", "DOCKER_BUILDKIT=1"} {
		c := NewCLI(t, WithEnv(env))

		t.Run(env+" build named and unnamed images", func(t *testing.T) {
			// ensure local test run does not reuse previously build image
			c.RunDockerOrExitError(t, "rmi", "-f", "build-test-nginx")
			c.RunDockerOrExitError(t, "rmi", "-f", "custom-nginx")

			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build")

			res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
			c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
			c.RunDockerCmd(t, "image", "inspect", "custom-nginx")
		})

		t.Run(env+" build with build-arg", func(t *testing.T) {
			// ensure local test run does not reuse previously build image
			c.RunDockerOrExitError(t, "rmi", "-f", "build-test-nginx")
			c.RunDockerOrExitError(t, "rmi", "-f", "custom-nginx")

			c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build", "--build-arg", "FOO=BAR")

			res := c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
			res.Assert(t, icmd.Expected{Out: `"FOO": "BAR"`})
		})

		t.Run(env+" build with build-arg set by env", func(t *testing.T) {
			// ensure local test run does not reuse previously build image
			c.RunDockerOrExitError(t, "rmi", "-f", "build-test-nginx")
			c.RunDockerOrExitError(t, "rmi", "-f", "custom-nginx")

			icmd.RunCmd(c.NewDockerComposeCmd(t,
				"--project-directory",
				"fixtures/build-test",
				"build",
				"--build-arg",
				"FOO"),
				func(cmd *icmd.Cmd) {
					cmd.Env = append(cmd.Env, "FOO=BAR")
				}).Assert(t, icmd.Success)

			res := c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
			res.Assert(t, icmd.Expected{Out: `"FOO": "BAR"`})
		})

		t.Run(env+" build with multiple build-args ", func(t *testing.T) {
			// ensure local test run does not reuse previously build image
			c.RunDockerOrExitError(t, "rmi", "-f", "multi-args-multiargs")
			cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/multi-args", "build")

			icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
				cmd.Env = append(cmd.Env, "DOCKER_BUILDKIT=0")
			})

			res := c.RunDockerCmd(t, "image", "inspect", "multi-args-multiargs")
			res.Assert(t, icmd.Expected{Out: `"RESULT": "SUCCESS"`})
		})

		t.Run(env+" build as part of up", func(t *testing.T) {
			// ensure local test run does not reuse previously build image
			c.RunDockerOrExitError(t, "rmi", "-f", "build-test-nginx")
			c.RunDockerOrExitError(t, "rmi", "-f", "custom-nginx")

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

		t.Run(env+" no rebuild when up again", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "up", "-d")

			assert.Assert(t, !strings.Contains(res.Stdout(), "COPY static"))
		})

		t.Run(env+" rebuild when up --build", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--workdir", "fixtures/build-test", "up", "-d", "--build")

			res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
			res.Assert(t, icmd.Expected{Out: "COPY static2 /usr/share/nginx/html"})
		})

		t.Run(env+" build --push ignored for unnamed images", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--workdir", "fixtures/build-test", "build", "--push", "nginx")
			assert.Assert(t, !strings.Contains(res.Stdout(), "failed to push"), res.Stdout())
		})

		t.Run(env+" cleanup build project", func(t *testing.T) {
			c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "down")
			c.RunDockerOrExitError(t, "rmi", "-f", "build-test-nginx")
			c.RunDockerOrExitError(t, "rmi", "-f", "custom-nginx")
		})
	}

}

func TestBuildSSH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Running on Windows. Skipping...")
	}
	c := NewParallelCLI(t)

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
			Err:      "unset ssh forward key fake-ssh",
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
}

func TestBuildSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on windows")
	}
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
			res := cli.RunDockerOrExitError(t, "image", "rm", "build-dependencies-service")
			if res.Error != nil {
				require.Contains(t, res.Stderr(), `No such image: build-dependencies-service`)
			}
		}
		resetState()
		t.Cleanup(resetState)

		// the image should NOT exist now
		res := cli.RunDockerOrExitError(t, "image", "inspect", "build-dependencies-service")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such image: build-dependencies-service",
		})

		res = cli.RunDockerComposeCmd(t, "build")
		t.Log(res.Combined())

		res = cli.RunDockerCmd(t,
			"image", "inspect", "--format={{ index .RepoTags 0 }}",
			"build-dependencies-service")
		res.Assert(t, icmd.Expected{Out: "build-dependencies-service:latest"})

		res = cli.RunDockerComposeCmd(t, "down", "-t0", "--rmi=all", "--remove-orphans")
		t.Log(res.Combined())

		res = cli.RunDockerOrExitError(t, "image", "inspect", "build-dependencies-service")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such image: build-dependencies-service",
		})
	}

	t.Run("ClassicBuilder", func(t *testing.T) {
		cli := NewCLI(t, WithEnv(
			"DOCKER_BUILDKIT=0",
			"COMPOSE_FILE=./fixtures/build-dependencies/compose.yaml",
		))
		doTest(t, cli)
	})

	t.Run("BuildKit", func(t *testing.T) {
		cli := NewCLI(t, WithEnv(
			"DOCKER_BUILDKIT=1",
			"COMPOSE_FILE=./fixtures/build-dependencies/compose.yaml",
		))
		doTest(t, cli)
	})
}

func TestBuildPlatformsWithCorrectBuildxConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Running on Windows. Skipping...")
	}
	c := NewParallelCLI(t)

	// declare builder
	result := c.RunDockerCmd(t, "buildx", "create", "--name", "build-platform", "--use", "--bootstrap")
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/platforms", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", "build-platform")
	})

	t.Run("platform not supported by builder", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-unsupported-platform.yml", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 17,
			Err:      "no match for platform in",
		})
	})

	t.Run("multi-arch build ok", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms", "build")
		assert.NilError(t, res.Error, res.Stderr())
		res.Assert(t, icmd.Expected{Out: "I am building for linux/arm64"})
		res.Assert(t, icmd.Expected{Out: "I am building for linux/amd64"})

	})

	t.Run("multi-arch multi service builds ok", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-multiple-platform-builds.yaml", "build")
		assert.NilError(t, res.Error, res.Stderr())
		res.Assert(t, icmd.Expected{Out: "I'm Service A and I am building for linux/arm64"})
		res.Assert(t, icmd.Expected{Out: "I'm Service A and I am building for linux/amd64"})
		res.Assert(t, icmd.Expected{Out: "I'm Service B and I am building for linux/arm64"})
		res.Assert(t, icmd.Expected{Out: "I'm Service B and I am building for linux/amd64"})
		res.Assert(t, icmd.Expected{Out: "I'm Service C and I am building for linux/arm64"})
		res.Assert(t, icmd.Expected{Out: "I'm Service C and I am building for linux/amd64"})
	})

	t.Run("multi-arch up --build", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms", "up", "--build", "--menu=false")
		assert.NilError(t, res.Error, res.Stderr())
		res.Assert(t, icmd.Expected{Out: "platforms-1 exited with code 0"})
	})

	t.Run("use DOCKER_DEFAULT_PLATFORM value when up --build", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/platforms", "up", "--build", "--menu=false")
		res := icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "DOCKER_DEFAULT_PLATFORM=linux/amd64")
		})
		assert.NilError(t, res.Error, res.Stderr())
		res.Assert(t, icmd.Expected{Out: "I am building for linux/amd64"})
		assert.Assert(t, !strings.Contains(res.Stdout(), "I am building for linux/arm64"))
	})

	t.Run("use service platform value when no build platforms defined ", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-service-platform-and-no-build-platforms.yaml", "build")
		assert.NilError(t, res.Error, res.Stderr())
		res.Assert(t, icmd.Expected{Out: "I am building for linux/386"})
	})

}

func TestBuildPrivileged(t *testing.T) {
	c := NewParallelCLI(t)

	// declare builder
	result := c.RunDockerCmd(t, "buildx", "create", "--name", "build-privileged", "--use", "--bootstrap", "--buildkitd-flags",
		`'--allow-insecure-entitlement=security.insecure'`)
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/privileged", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", "build-privileged")
	})

	t.Run("use build privileged mode to run insecure build command", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/privileged", "build")
		capEffRe := regexp.MustCompile("CapEff:\t([0-9a-f]+)")
		matches := capEffRe.FindStringSubmatch(res.Stdout())
		assert.Equal(t, 2, len(matches), "Did not match CapEff in output, matches: %v", matches)

		capEff, err := strconv.ParseUint(matches[1], 16, 64)
		assert.NilError(t, err, "Parsing CapEff: %s", matches[1])

		// NOTE: can't use constant from x/sys/unix or tests won't compile on macOS/Windows
		// #define CAP_SYS_ADMIN        21
		// https://github.com/torvalds/linux/blob/v6.1/include/uapi/linux/capability.h#L278
		const capSysAdmin = 0x15
		if capEff&capSysAdmin != capSysAdmin {
			t.Fatalf("CapEff %s is missing CAP_SYS_ADMIN", matches[1])
		}
	})
}

func TestBuildPlatformsStandardErrors(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("no platform support with Classic Builder", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/platforms", "build")

		res := icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "DOCKER_BUILDKIT=0")
		})
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "the classic builder doesn't support multi-arch build, set DOCKER_BUILDKIT=1 to use BuildKit",
		})
	})

	t.Run("builder does not support multi-arch", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 17,
			Err: `Multi-platform build is not supported for the docker driver.
Switch to a different driver, or turn on the containerd image store, and try again.`,
		})
	})

	t.Run("service platform not defined in platforms build section", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-service-platform-not-in-build-platforms.yaml", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 15,
			Err:      `service.build.platforms MUST include service.platform "linux/riscv64"`,
		})
	})

	t.Run("DOCKER_DEFAULT_PLATFORM value not defined in platforms build section", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/platforms", "build")
		res := icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "DOCKER_DEFAULT_PLATFORM=windows/amd64")
		})
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      `service "platforms" build.platforms does not support value set by DOCKER_DEFAULT_PLATFORM: windows/amd64`,
		})
	})

	t.Run("no privileged support with Classic Builder", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "--project-directory", "fixtures/build-test/privileged", "build")

		res := icmd.RunCmd(cmd, func(cmd *icmd.Cmd) {
			cmd.Env = append(cmd.Env, "DOCKER_BUILDKIT=0")
		})
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "the classic builder doesn't support privileged mode, set DOCKER_BUILDKIT=1 to use BuildKit",
		})
	})

}

func TestBuildBuilder(t *testing.T) {
	c := NewParallelCLI(t)
	builderName := "build-with-builder"
	// declare builder
	result := c.RunDockerCmd(t, "buildx", "create", "--name", builderName, "--use", "--bootstrap")
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", builderName)
	})

	t.Run("use specific builder to run build command", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test", "build", "--builder", builderName)
		assert.NilError(t, res.Error, res.Stderr())
	})

	t.Run("error when using specific builder to run build command", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test", "build", "--builder", "unknown-builder")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      fmt.Sprintf(`no builder %q found`, "unknown-builder"),
		})
	})

}

func TestBuildEntitlements(t *testing.T) {
	c := NewParallelCLI(t)

	// declare builder
	result := c.RunDockerCmd(t, "buildx", "create", "--name", "build-insecure", "--use", "--bootstrap", "--buildkitd-flags",
		`'--allow-insecure-entitlement=security.insecure'`)
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/entitlements", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", "build-insecure")
	})

	t.Run("use build privileged mode to run insecure build command", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/entitlements", "build")
		capEffRe := regexp.MustCompile("CapEff:\t([0-9a-f]+)")
		matches := capEffRe.FindStringSubmatch(res.Stdout())
		assert.Equal(t, 2, len(matches), "Did not match CapEff in output, matches: %v", matches)

		capEff, err := strconv.ParseUint(matches[1], 16, 64)
		assert.NilError(t, err, "Parsing CapEff: %s", matches[1])

		// NOTE: can't use constant from x/sys/unix or tests won't compile on macOS/Windows
		// #define CAP_SYS_ADMIN        21
		// https://github.com/torvalds/linux/blob/v6.1/include/uapi/linux/capability.h#L278
		const capSysAdmin = 0x15
		if capEff&capSysAdmin != capSysAdmin {
			t.Fatalf("CapEff %s is missing CAP_SYS_ADMIN", matches[1])
		}
	})
}
