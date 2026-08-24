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
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestLocalComposeBuild(t *testing.T) {
	for _, env := range []string{"DOCKER_BUILDKIT=0", "DOCKER_BUILDKIT=1"} {
		c := NewCLI(t, WithEnv(strings.Split(env, ",")...))

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

			output := HTTPGetWithRetry(t, fmt.Sprintf("http://localhost:%d", c.ServicePublishedPort(t, "build-test", "nginx", 80)), http.StatusOK, 2*time.Second, 20*time.Second)
			assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))

			c.RunDockerCmd(t, "image", "inspect", "build-test-nginx")
			c.RunDockerCmd(t, "image", "inspect", "custom-nginx")
		})

		t.Run(env+" no rebuild when up again", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "up", "-d")

			assert.Assert(t, !strings.Contains(res.Stdout(), "COPY static"))
		})

		t.Run(env+" rebuild when up --build", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "up", "-d", "--build")

			res.Assert(t, icmd.Expected{Out: "COPY static /usr/share/nginx/html"})
			res.Assert(t, icmd.Expected{Out: "COPY static2 /usr/share/nginx/html"})
		})

		t.Run(env+" build --push ignored for unnamed images", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build", "--push", "nginx")
			assert.Assert(t, !strings.Contains(res.Stdout(), "failed to push"), res.Stdout())
		})

		t.Run(env+" build --quiet", func(t *testing.T) {
			res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test", "build", "--quiet")
			res.Assert(t, icmd.Expected{Out: ""})
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
			ExitCode: 1,
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
	// the Dockerfile diffs each mounted secret against its expected value, so
	// a successful build proves file, environment and .env secrets all reached
	// the build.
	s := NewScenario(t, "build secrets from a file, the environment and the .env must reach the build")
	image := s.Project() + "-secret"
	s.Env("SECRET_IMAGE="+image).
		Defer(DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("build verifies each secret's content in-Dockerfile",
			ComposeCmd("build").WithEnv("SOME_SECRET=bar"),
			ImageExists(image))
}

func TestBuildTags(t *testing.T) {
	s := NewScenario(t, "build must apply every declared tag alongside the service image name")
	image := s.Project() + "-tags"
	s.Env("TAG_IMAGE="+image).
		Defer(
			DockerCmd("image", "rm", "-f", image).MayFail(),
			DockerCmd("image", "rm", "-f", "docker/"+image+":1.0.0").MayFail(),
			DockerCmd("image", "rm", "-f", image+"-other:v1.0.0").MayFail()).
		Step("build tags the image under every name",
			ComposeCmd("build", "--no-cache")).
		Step("the image carries the three tags",
			DockerCmd("image", "inspect", image),
			OutputContains("docker/"+image+":1.0.0"),
			OutputContains(image+":latest"),
			OutputContains(image+"-other:v1.0.0"))
}

func TestBuildImageDependencies(t *testing.T) {
	doTest := func(t *testing.T, cli *CLI, args ...string) {
		resetState := func() {
			cli.RunDockerComposeCmd(t, "down", "--rmi=all", "-t=0")
			res := cli.RunDockerOrExitError(t, "image", "rm", "build-dependencies-service")
			if res.Error != nil {
				assert.Assert(t, is.Contains(res.Stderr(), `No such image: build-dependencies-service`))
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

		res = cli.RunDockerComposeCmd(t, args...)
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
			"COMPOSE_FILE=./fixtures/build-dependencies/classic.yaml",
		))
		doTest(t, cli, "build")
		doTest(t, cli, "build", "--with-dependencies", "service")
	})

	t.Run("Bake by additional contexts", func(t *testing.T) {
		cli := NewCLI(t, WithEnv(
			"DOCKER_BUILDKIT=1", "COMPOSE_BAKE=1",
			"COMPOSE_FILE=./fixtures/build-dependencies/compose.yaml",
		))
		doTest(t, cli, "--verbose", "build")
		doTest(t, cli, "--verbose", "build", "service")
		doTest(t, cli, "--verbose", "up", "--build", "service")
	})
}

func TestBuildPlatformsWithCorrectBuildxConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Running on Windows. Skipping...")
	}
	c := NewParallelCLI(t)

	// declare a per-test-unique builder to avoid container-name collisions
	// when tests run in parallel on the same Docker daemon.
	builderName := BuilderName(t, "build-platform")
	result := c.RunDockerCmd(t, "buildx", "create", "--name", builderName, "--use", "--bootstrap")
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/platforms", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", builderName)
	})

	t.Run("platform not supported by builder", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-unsupported-platform.yml", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "no match for platform",
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

	// declare a per-test-unique builder to avoid container-name collisions
	// when tests run in parallel on the same Docker daemon.
	builderName := BuilderName(t, "build-privileged")
	result := c.RunDockerCmd(t, "buildx", "create", "--name", builderName, "--use", "--bootstrap", "--buildkitd-flags",
		`'--allow-insecure-entitlement=security.insecure'`)
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/privileged", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", builderName)
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
		// The docker driver supports multi-platform builds whenever the
		// daemon uses the containerd image store, so this error won't occur.
		// Detect the store directly: the buildx `Platforms:` heuristic below
		// misses it on hosts without binfmt emulation, where only native
		// platforms are listed even though cross-building works.
		info := c.RunDockerCmd(t, "info", "-f", "{{json .DriverStatus}}")
		if strings.Contains(info.Stdout(), "io.containerd.snapshotter.v1") {
			t.Skip("docker driver supports multi-platform builds (containerd image store enabled)")
		}
		// Docker Desktop with containerd image store uses the docker driver
		// but supports multi-platform builds, so this error won't occur.
		inspect := c.RunDockerCmd(t, "buildx", "inspect", "--bootstrap")
		output := inspect.Stdout()
		isDockerDriver := false
		platforms := ""
		for line := range strings.SplitSeq(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(trimmed, "Driver:"); ok {
				isDockerDriver = strings.TrimSpace(after) == "docker"
			}
			if strings.HasPrefix(trimmed, "Platforms:") {
				platforms = trimmed
			}
		}
		if isDockerDriver && strings.Contains(platforms, "linux/amd64") && strings.Contains(platforms, "linux/arm64") {
			t.Skip("docker driver supports multi-platform (containerd image store enabled)")
		}

		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "Multi-platform build is not supported for the docker driver.",
		})
	})

	t.Run("service platform not defined in platforms build section", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/build-test/platforms",
			"-f", "fixtures/build-test/platforms/compose-service-platform-not-in-build-platforms.yaml", "build")
		res.Assert(t, icmd.Expected{
			ExitCode: 1,
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
	// declare a per-test-unique builder to avoid container-name collisions
	// when tests run in parallel on the same Docker daemon.
	builderName := BuilderName(t, "build-with-builder")
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

	// declare a per-test-unique builder to avoid container-name collisions
	// when tests run in parallel on the same Docker daemon.
	builderName := BuilderName(t, "build-insecure")
	result := c.RunDockerCmd(t, "buildx", "create", "--name", builderName, "--use", "--bootstrap", "--buildkitd-flags",
		`'--allow-insecure-entitlement=security.insecure'`)
	assert.NilError(t, result.Error)

	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/build-test/entitlements", "down")
		_ = c.RunDockerCmd(t, "buildx", "rm", "-f", builderName)
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

func TestBuildDependsOn(t *testing.T) {
	s := NewScenario(t, "up must build a pull_policy: build dependency before starting its dependent")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-test1").MayFail()).
		Step("up on the dependent reports the dependency's build",
			ComposeCmd("--progress=plain", "up", "test2"),
			OutputContains("test1 Built"))
}

func TestBuildSubset(t *testing.T) {
	s := NewScenario(t, "build scoped to a service must build that service")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-main").MayFail()).
		Step("build main reports it built",
			ComposeCmd("build", "main"),
			OutputContains("main Built"))
}

func TestBuildDependentImage(t *testing.T) {
	s := NewScenario(t, "each service using another service's image as build context must build on demand")
	s.Defer(
		DockerCmd("image", "rm", "-f", s.Project()+"-firstbuild").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-secondbuild").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-dep1").MayFail()).
		Step("the first dependent builds",
			ComposeCmd("build", "firstbuild"),
			OutputContains("firstbuild Built")).
		Step("the second dependent builds too",
			ComposeCmd("build", "secondbuild"),
			OutputContains("secondbuild Built"))
}

func TestBuildSubDependencies(t *testing.T) {
	s := NewScenario(t, "a chain of service build contexts must resolve transitively, for build and up --build alike")
	s.Defer(
		DockerCmd("image", "rm", "-f", s.Project()+"-main").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-dep1").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-dep2").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-subdep1").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-subdep2").MayFail()).
		Step("build resolves the whole context chain",
			ComposeCmd("build", "main"),
			OutputContains("main Built")).
		Step("up --build resolves it the same way",
			ComposeCmd("up", "--build", "main"),
			OutputContains("main Built"))
}

func TestBuildLongOutputLine(t *testing.T) {
	s := NewScenario(t, "a build flooding the progress writer with warnings must still complete and report")
	s.
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-long-line").MayFail()).
		Step("build survives the warning flood",
			ComposeCmd("build", "long-line"),
			OutputContains("long-line Built")).
		Step("up --build does too",
			ComposeCmd("up", "--build", "long-line"),
			OutputContains("long-line Built"))
}

func TestBuildDependentImageWithProfile(t *testing.T) {
	s := NewScenario(t, "build targeting a profiled service must activate its profile and mount its build secret")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-secret-build-test").MayFail()).
		Step("build reports the profiled service built",
			ComposeCmd("build", "secret-build-test"),
			OutputContains("secret-build-test Built"))
}

func TestBuildTLS(t *testing.T) {
	t.Helper()

	c := NewParallelCLI(t)
	// Use a per-test-unique name to avoid container/context collisions when
	// tests run in parallel on the same Docker daemon.
	dindBuilder := BuilderName(t, "e2e-dind-builder")
	tmp := t.TempDir()

	t.Cleanup(func() {
		c.RunDockerCmd(t, "rm", "-f", dindBuilder)
		c.RunDockerCmd(t, "context", "rm", dindBuilder)
	})

	c.RunDockerCmd(t, "run", "--name", dindBuilder, "--privileged", "-p", "127.0.0.1::2376", "-d", "docker:dind")

	poll.WaitOn(t, func(_ poll.LogT) poll.Result {
		res := c.RunDockerCmd(t, "logs", dindBuilder)
		if strings.Contains(res.Combined(), "API listen on [::]:2376") {
			return poll.Success()
		}
		return poll.Continue("waiting for Docker daemon to be running")
	}, poll.WithTimeout(10*time.Second))

	time.Sleep(1 * time.Second) // wait for dind setup
	c.RunDockerCmd(t, "cp", dindBuilder+":/certs/client", tmp)

	res := c.RunDockerCmd(t, "inspect", "-f", "{{(index (index .NetworkSettings.Ports \"2376/tcp\") 0).HostPort}}", dindBuilder)
	hostPort := strings.TrimSpace(res.Stdout())
	if hostPort == "" {
		t.Fatal("failed to resolve mapped host port for 2376/tcp")
	}

	c.RunDockerCmd(t, "context", "create", dindBuilder, "--docker",
		fmt.Sprintf("host=tcp://127.0.0.1:%s,ca=%s/client/ca.pem,cert=%s/client/cert.pem,key=%s/client/key.pem,skip-tls-verify=1", hostPort, tmp, tmp, tmp))

	cmd := c.NewDockerComposeCmd(t, "-f", "fixtures/build-test/minimal/compose.yaml", "build")
	cmd.Env = append(cmd.Env, "DOCKER_CONTEXT="+dindBuilder)
	cmd.Stdout = os.Stdout
	res = icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{Err: "Built"})
}

func TestBuildEscaped(t *testing.T) {
	s := NewScenario(t, "a $$ escape in the model must reach the build literally, in args, heredocs and inline dockerfiles")
	s.Defer(
		DockerCmd("image", "rm", "-f", s.Project()+"-foo").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-echo").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-arg").MayFail()).
		Step("the escaped build arg reaches the Dockerfile literally",
			ComposeCmd("build", "--no-cache", "foo"),
			OutputContains("foo is ${bar}")).
		Step("a heredoc with command substitution builds",
			ComposeCmd("build", "--no-cache", "echo")).
		Step("an escaped variable in an inline dockerfile builds",
			ComposeCmd("build", "--no-cache", "arg"))
}

// TestUpBuildUnchangedContext locks the invariant that rebuilding an
// unchanged build context hits the build cache and leaves the running
// service alone: same image, same config hash, same container.
// Its build context lives in testdata/TestUpBuildUnchangedContext/.
func TestUpBuildUnchangedContext(t *testing.T) {
	s := NewScenario(t, "an unchanged up --build must hit the build cache and not recreate the service")
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-app")).
		Step("up builds the image and starts the service",
			ComposeCmd("up", "-d", "--build"),
			ServiceState("app", "running")).
		Step("an unchanged up --build reuses the cached image and the container",
			ComposeCmd("up", "-d", "--build"),
			NotRecreated("app"),
			LabelUnchanged("app", "com.docker.compose.config-hash"))
}
