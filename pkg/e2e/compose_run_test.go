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
	"time"

	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestComposeRun(t *testing.T) {
	s := NewScenario(t, "run must execute a one-off with the service command or an override, starting its dependencies")
	s.Step("run executes the service's own command and starts its dependency",
		ComposeCmd("run", "back"),
		StdoutContains("Hello there!!"),
		OutputNotContains("orphan"),
		ServiceState("db", "running"),
		OneOffState("back", "exited"),
		ServiceNotCreated("front")).
		Step("run with an override command warns about the previous one-off, now an orphan",
			ComposeCmd("run", "back", "echo", "Hello one more time"),
			StdoutContains("Hello one more time"),
			OutputContains("orphan")).
		Step("COMPOSE_IGNORE_ORPHANS silences the warning",
			ComposeCmd("run", "back", "echo", "Hello again").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			StdoutContains("Hello again"),
			OutputNotContains("orphan")).
		Step("run --rm leaves the earlier one-offs alone and removes its own container",
			ComposeCmd("run", "--rm", "back", "echo", "Hello and gone").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			StdoutContains("Hello and gone"),
			OneOffsUntouched("back")).
		Step("run --volumes bind-mounts the requested host path",
			ComposeCmd("run", "--volumes", s.Dir()+":/foo", "back", "/bin/sh", "-c", "ls /foo").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			StdoutContains("compose.yaml")).
		Step("run --env-from-file injects the file's variables",
			ComposeCmd("run", "--env-from-file", s.Dir()+"/run.env", "front", "env").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			StdoutContains("FOO=BAR")).
		Step("run --env injects the variable",
			ComposeCmd("run", "--env", "FOO=BAR", "front", "env").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			StdoutContains("FOO=BAR"))
}

func TestComposeRunPorts(t *testing.T) {
	s := NewScenario(t, "run must only publish ports when asked: --publish for ad-hoc, --service-ports for the model's")
	s.Step("run --publish maps the requested port, not the model's",
		ComposeCmd("run", "--publish", "8081:80", "-d", "back", "/bin/sh", "-c", "sleep 30")).
		Step("the ad-hoc mapping is live",
			DockerCmd("ps", "--filter", "label=com.docker.compose.project="+s.Project()),
			OutputContains("8081->80/tcp"),
			OutputNotContains("8082->80/tcp")).
		Step("run --service-ports maps the model's ports",
			ComposeCmd("run", "--service-ports", "-d", "back", "/bin/sh", "-c", "sleep 30")).
		Step("the model's mapping is live",
			DockerCmd("ps", "--filter", "label=com.docker.compose.project="+s.Project()),
			OutputContains("8082->80/tcp"))
}

func TestComposeRunDeps(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/9459
	// run used to start other services of the project beyond the target's
	// dependency chain.
	NewScenario(t, "run must start the target's dependencies and nothing else, unless --no-deps").
		Step("run starts the shared dependency but not the sibling service",
			ComposeCmd("run", "service_a"),
			OutputContains("shared_dep"),
			OutputNotContains("service_b"),
			ServiceNotCreated("service_b")).
		Step("run --no-deps starts nothing but the one-off",
			ComposeCmd("run", "--no-deps", "service_a").WithEnv("COMPOSE_IGNORE_ORPHANS=True"),
			OutputNotContains("service_b"),
			OutputNotContains("shared_dep"),
			NotRecreated("shared_dep"),
			ServiceNotCreated("service_b"))
}

func TestComposeRunNotRequiredDeps(t *testing.T) {
	NewScenario(t, "run must skip a dependency marked required: false when its profile is inactive").
		Step("run executes the service without materializing the optional dependency",
			ComposeCmd("run", "foo"),
			OutputContains("foo"),
			ServiceNotCreated("bar"))
}

func TestComposeRunQuietPull(t *testing.T) {
	NewScenario(t, "run --quiet-pull and COMPOSE_PROGRESS=quiet must silence pull progress at two levels").
		Step("start without the image locally",
			ComposeCmd("down", "--rmi", "all")).
		Step("--quiet-pull keeps the decision but drops the layer progress",
			ComposeCmd("run", "--quiet-pull", "backend"),
			OutputNotContains("Pull complete"),
			OutputContains("Pulled")).
		Step("remove the image again",
			ComposeCmd("down", "--rmi", "all")).
		Step("COMPOSE_PROGRESS=quiet silences the pull entirely",
			ComposeCmd("run", "backend").WithEnv("COMPOSE_PROGRESS=quiet"),
			OutputNotContains("Pull complete"),
			OutputNotContains("Pulled"))
}

func TestComposeRunPullAlways(t *testing.T) {
	NewScenario(t, "run --pull always must pull the image even when present locally").
		Step("run reports the pull it was asked to always perform",
			ComposeCmd("run", "--pull", "always", "backend"),
			OutputContains("Image nginx Pulling"),
			OutputContains("Image nginx Pulled"))
}

func TestComposeRunBuild(t *testing.T) {
	s := NewScenario(t, "run must build a service whose image comes from another service's build context")
	s.Defer(
		DockerCmd("image", "rm", "-f", s.Project()+"-build").MayFail(),
		DockerCmd("image", "rm", "-f", s.Project()+"-build_base").MayFail()).
		Step("run builds the chained images and executes the command",
			ComposeCmd("run", "build", "echo", "hello world"),
			StdoutContains("hello world"))
}

func TestComposeRunRmStopSignal(t *testing.T) {
	c := NewParallelCLI(t)
	projectName := "run-test"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})

	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "-f", "./fixtures/ps-test/compose.yaml", "run", "--rm", "-d", "nginx")
	res.Assert(t, icmd.Success)

	res = c.RunDockerCmd(t, "ps", "--quiet", "--filter", "name=run-test-nginx")
	containerID := strings.TrimSpace(res.Stdout())

	res = c.RunDockerCmd(t, "stop", containerID)
	res.Assert(t, icmd.Success)
	// --rm auto-removal is async, wait for the container to be removed
	poll.WaitOn(t, func(l poll.LogT) poll.Result {
		res = c.RunDockerCmd(t, "ps", "--all", "--filter", "name=run-test-nginx", "--format", "'{{.Names}}'")
		if strings.Contains(res.Stdout(), "run-test-nginx") {
			return poll.Continue("container still present: %s", res.Stdout())
		}
		return poll.Success()
	}, poll.WithTimeout(10*time.Second), poll.WithDelay(500*time.Millisecond))
}

func TestComposeRunPipedInput(t *testing.T) {
	if composeStandaloneMode {
		t.Skip("Skipping test compose with piped input detection in standalone mode")
	}
	c := NewParallelCLI(t)
	defer c.cleanupWithDown(t, "run-piped-test")

	t.Run("compose run with piped input detection", func(t *testing.T) {
		// Test that piped input is properly detected and TTY is automatically disabled
		// This tests the logic added in run.go that checks dockerCli.In().IsTerminal()
		cmd := c.NewCmd("sh", "-c", "echo 'piped-content' | docker compose -p run-piped-test -f ./fixtures/run-test/piped-test.yaml run --rm piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{Out: "piped-content"})
		res.Assert(t, icmd.Success)
	})

	t.Run("compose run piped input should not allocate TTY", func(t *testing.T) {
		// Test that when stdin is piped, the container correctly detects no TTY
		// This verifies that the automatic noTty=true setting works correctly
		cmd := c.NewCmd("sh", "-c", "echo '' | docker compose -p run-piped-test -f ./fixtures/run-test/piped-test.yaml run --rm tty-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{Out: "No TTY detected"})
		res.Assert(t, icmd.Success)
	})

	t.Run("compose run piped input with explicit --tty should fail", func(t *testing.T) {
		// Test that explicitly requesting TTY with piped input fails with proper error message
		// This should trigger the "input device is not a TTY" error
		cmd := c.NewCmd("sh", "-c", "echo 'test' | docker compose -p run-piped-test -f ./fixtures/run-test/piped-test.yaml run --rm --tty piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "cannot attach stdin to a TTY-enabled container because stdin is not a terminal",
		})
	})

	t.Run("compose run piped input with --no-tty=false should fail", func(t *testing.T) {
		// Test that explicitly disabling --no-tty (i.e., requesting TTY) with piped input fails
		// This should also trigger the "input device is not a TTY" error
		cmd := c.NewCmd("sh", "-c", "echo 'test' | docker compose -p run-piped-test -f ./fixtures/run-test/piped-test.yaml run --rm --no-tty=false piped-test")
		res := icmd.RunCmd(cmd)

		res.Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "cannot attach stdin to a TTY-enabled container because stdin is not a terminal",
		})
	})
}
