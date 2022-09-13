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

	testify "github.com/stretchr/testify/assert"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestStartStop(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-stop-no-dependencies"

	getProjectRegx := func(status string) string {
		// match output with random spaces like:
		// e2e-start-stop      running(3)
		return fmt.Sprintf("%s\\s+%s\\(%d\\)", projectName, status, 2)
	}

	getServiceRegx := func(service string, status string) string {
		// match output with random spaces like:
		// e2e-start-stop-db-1      "echo hello"       db          running
		return fmt.Sprintf("%s-%s-1.+%s\\s+%s", projectName, service, service, status)
	}

	t.Run("Up a project", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "up",
			"-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-no-dependencies-simple-1  Started"), res.Combined())

		res = c.RunDockerComposeCmd(t, "ls", "--all")
		testify.Regexp(t, getProjectRegx("running"), res.Stdout())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps")
		testify.Regexp(t, getServiceRegx("simple", "running"), res.Stdout())
		testify.Regexp(t, getServiceRegx("another", "running"), res.Stdout())
	})

	t.Run("stop project", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "stop")

		res := c.RunDockerComposeCmd(t, "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-no-dependencies"), res.Combined())

		res = c.RunDockerComposeCmd(t, "ls", "--all")
		testify.Regexp(t, getProjectRegx("exited"), res.Stdout())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-no-dependencies-words-1"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--all")
		testify.Regexp(t, getServiceRegx("simple", "exited"), res.Stdout())
		testify.Regexp(t, getServiceRegx("another", "exited"), res.Stdout())
	})

	t.Run("start project", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "--project-name", projectName, "start")

		res := c.RunDockerComposeCmd(t, "ls")
		testify.Regexp(t, getProjectRegx("running"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestStartStopWithDependencies(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-stop-with-dependencies"

	defer c.RunDockerComposeCmd(t, "--project-name", projectName, "rm", "-fsv")

	t.Run("Up", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "--project-name", projectName,
			"up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-foo-1  Started"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-bar-1  Started"), res.Combined())
	})

	t.Run("stop foo", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop", "foo")

		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-foo-1  Stopped"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--status", "running")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-dependencies-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-dependencies-foo-1"), res.Combined())
	})

	t.Run("start foo", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-bar-1  Stopped"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "start", "foo")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-bar-1  Started"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-foo-1  Started"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "--status", "running")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-dependencies-bar-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-dependencies-foo-1"), res.Combined())
	})

	t.Run("Up no-deps links", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/links/compose.yaml", "--project-name", projectName, "up",
			"--no-deps", "-d", "foo")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-foo-1  Started"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "Container e2e-start-stop-with-dependencies-bar-1  Started"), res.Combined())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestStartStopWithOneOffs(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-stop-with-oneoffs"

	t.Run("Up", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "--project-name", projectName,
			"up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-oneoffs-foo-1  Started"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-oneoffs-bar-1  Started"), res.Combined())
	})

	t.Run("run one-off", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "--project-name", projectName, "run", "-d", "bar", "sleep", "infinity")
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs_bar_run"), res.Combined())
	})

	t.Run("stop (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e_start_stop_with_oneoffs_bar_run"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a", "--status", "running")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs_bar_run"), res.Combined())
	})

	t.Run("start (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "start")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs_bar_run"), res.Combined())
	})

	t.Run("restart (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "restart")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs_bar_run"), res.Combined())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--remove-orphans")

		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a", "--status", "running")
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar"), res.Combined())
	})
}

func TestStartAlreadyRunning(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-start-stop-svc-already-running",
		"COMPOSE_FILE=./fixtures/start-stop/compose.yaml"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
	})

	cli.RunDockerComposeCmd(t, "up", "-d", "--wait")

	res := cli.RunDockerComposeCmd(t, "start", "simple")
	assert.Equal(t, res.Stdout(), "", "No output should have been written to stdout")
}

func TestStopAlreadyStopped(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-start-stop-svc-already-stopped",
		"COMPOSE_FILE=./fixtures/start-stop/compose.yaml"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
	})

	cli.RunDockerComposeCmd(t, "up", "-d", "--wait")

	// stop the container
	cli.RunDockerComposeCmd(t, "stop", "simple")

	// attempt to stop it again
	res := cli.RunDockerComposeCmdNoCheck(t, "stop", "simple")
	// TODO: for consistency, this should NOT write any output because the
	// 		container is already stopped
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Err:      "Container e2e-start-stop-svc-already-stopped-simple-1  Stopped",
	})
}

func TestStartStopMultipleServices(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-start-stop-svc-multiple",
		"COMPOSE_FILE=./fixtures/start-stop/compose.yaml"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
	})

	cli.RunDockerComposeCmd(t, "up", "-d", "--wait")

	res := cli.RunDockerComposeCmd(t, "stop", "simple", "another")
	services := []string{"simple", "another"}
	for _, svc := range services {
		stopMsg := fmt.Sprintf("Container e2e-start-stop-svc-multiple-%s-1  Stopped", svc)
		assert.Assert(t, strings.Contains(res.Stderr(), stopMsg),
			fmt.Sprintf("Missing stop message for %s\n%s", svc, res.Combined()))
	}

	res = cli.RunDockerComposeCmd(t, "start", "simple", "another")
	for _, svc := range services {
		startMsg := fmt.Sprintf("Container e2e-start-stop-svc-multiple-%s-1  Started", svc)
		assert.Assert(t, strings.Contains(res.Stderr(), startMsg),
			fmt.Sprintf("Missing start message for %s\n%s", svc, res.Combined()))
	}
}

func TestStartSingleServiceAndDependency(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=e2e-start-single-deps",
		"COMPOSE_FILE=./fixtures/start-stop/start-stop-deps.yaml"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "down", "--remove-orphans", "-v", "-t", "0")
	})

	cli.RunDockerComposeCmd(t, "create", "desired")

	res := cli.RunDockerComposeCmd(t, "start", "desired")
	desiredServices := []string{"desired", "dep_1", "dep_2"}
	for _, s := range desiredServices {
		startMsg := fmt.Sprintf("Container e2e-start-single-deps-%s-1  Started", s)
		assert.Assert(t, strings.Contains(res.Combined(), startMsg),
			fmt.Sprintf("Missing start message for service: %s\n%s", s, res.Combined()))
	}
	undesiredServices := []string{"another", "another_2"}
	for _, s := range undesiredServices {
		assert.Assert(t, !strings.Contains(res.Combined(), s),
			fmt.Sprintf("Shouldn't have message for service: %s\n%s", s, res.Combined()))
	}
}

func TestStartStopMultipleFiles(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv("COMPOSE_PROJECT_NAME=e2e-start-stop-svc-multiple-files"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "-p", "e2e-start-stop-svc-multiple-files", "down", "--remove-orphans")
	})

	cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "up", "-d")
	cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/other.yaml", "up", "-d")

	res := cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "stop")
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-simple-1  Stopped"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-another-1  Stopped"), res.Combined())
	assert.Assert(t, !strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-a-different-one-1  Stopped"), res.Combined())
	assert.Assert(t, !strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-and-another-one-1  Stopped"), res.Combined())
}
