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

func TestStartStop(t *testing.T) {
	s := NewScenario(t, "stop must halt the project's containers in place, start must bring the same ones back")
	s.Step("up starts every service",
		ComposeCmd("up", "-d"),
		ServiceState("simple", "running"),
		ServiceState("another", "running")).
		Step("ls reports the project as running",
			ComposeCmd("ls", "--format", "json"),
			OutputContains(`"Name":"`+s.Project()+`"`)).
		Step("stop halts the containers without removing them",
			ComposeCmd("stop"),
			ServiceState("simple", "exited"),
			ServiceState("another", "exited")).
		Step("a stopped project is hidden from ls",
			ComposeCmd("ls", "--format", "json"),
			OutputNotContains(`"Name":"`+s.Project()+`"`)).
		Step("ls --all still lists the stopped project",
			ComposeCmd("ls", "--all", "--format", "json"),
			OutputContains(`"Name":"`+s.Project()+`"`)).
		Step("start brings the same containers back",
			ComposeCmd("start"),
			ServiceState("simple", "running"),
			ServiceState("another", "running"),
			NotRecreated("simple", "another"))
}

func TestStartStopWithDependencies(t *testing.T) {
	NewScenario(t, "stop must only halt the requested service, start must also start its dependencies").
		Step("up starts the service and its dependency",
			ComposeCmd("up", "-d"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running")).
		Step("stop foo leaves the dependency running",
			ComposeCmd("stop", "foo"),
			ServiceState("foo", "exited"),
			ServiceState("bar", "running")).
		Step("stop halts the whole project",
			ComposeCmd("stop"),
			ServiceState("foo", "exited"),
			ServiceState("bar", "exited")).
		Step("start foo brings its dependency back too",
			ComposeCmd("start", "foo"),
			ServiceState("foo", "running"),
			ServiceState("bar", "running"),
			NotRecreated("foo", "bar"))
}

func TestUpNoDeps(t *testing.T) {
	NewScenario(t, "up --no-deps must not create the service's dependencies").
		Step("up --no-deps starts only the requested service",
			ComposeCmd("up", "--no-deps", "-d", "foo"),
			ServiceState("foo", "running"),
			ServiceNotCreated("bar"))
}

func TestStartStopWithOneOffs(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-stop-with-oneoffs"

	t.Run("Up", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "--project-name", projectName,
			"up", "-d")
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-oneoffs-foo-1 Started"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-with-oneoffs-bar-1 Started"), res.Combined())
	})

	t.Run("run one-off", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "--project-name", projectName, "run", "-d", "bar", "sleep", "infinity")
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-run"), res.Combined())
	})

	t.Run("stop (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e_start_stop_with_oneoffs-bar-run"), res.Combined())

		res = c.RunDockerComposeCmd(t, "--project-name", projectName, "ps", "-a", "--status", "running")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-run"), res.Combined())
	})

	t.Run("start (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "start")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-run"), res.Combined())
	})

	t.Run("restart (not one-off containers)", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "restart")
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-foo-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-1"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "e2e-start-stop-with-oneoffs-bar-run"), res.Combined())
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
		Err:      "Container e2e-start-stop-svc-already-stopped-simple-1 Stopped",
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
		stopMsg := fmt.Sprintf("Container e2e-start-stop-svc-multiple-%s-1 Stopped", svc)
		assert.Assert(t, strings.Contains(res.Stderr(), stopMsg),
			fmt.Sprintf("Missing stop message for %s\n%s", svc, res.Combined()))
	}

	res = cli.RunDockerComposeCmd(t, "start", "simple", "another")
	for _, svc := range services {
		startMsg := fmt.Sprintf("Container e2e-start-stop-svc-multiple-%s-1 Started", svc)
		assert.Assert(t, strings.Contains(res.Stderr(), startMsg),
			fmt.Sprintf("Missing start message for %s\n%s", svc, res.Combined()))
	}
}

func TestStartSingleServiceAndDependency(t *testing.T) {
	NewScenario(t, "start of a created service must start its dependency chain and nothing else").
		Step("create prepares the desired service and its dependencies only",
			ComposeCmd("create", "desired"),
			ServiceState("desired", "created"),
			ServiceState("dep_1", "created"),
			ServiceState("dep_2", "created"),
			ServiceNotCreated("another"),
			ServiceNotCreated("another_2")).
		Step("start brings up the desired service with its dependencies, nothing else",
			ComposeCmd("start", "desired"),
			ServiceState("desired", "running"),
			ServiceState("dep_1", "running"),
			ServiceState("dep_2", "running"),
			ServiceNotCreated("another"),
			ServiceNotCreated("another_2"))
}

func TestStartStopMultipleFiles(t *testing.T) {
	cli := NewParallelCLI(t, WithEnv("COMPOSE_PROJECT_NAME=e2e-start-stop-svc-multiple-files"))
	t.Cleanup(func() {
		cli.RunDockerComposeCmd(t, "-p", "e2e-start-stop-svc-multiple-files", "down", "--remove-orphans")
	})

	cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "up", "-d")
	cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/other.yaml", "up", "-d")

	res := cli.RunDockerComposeCmd(t, "-f", "./fixtures/start-stop/compose.yaml", "stop")
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-simple-1 Stopped"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-another-1 Stopped"), res.Combined())
	assert.Assert(t, !strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-a-different-one-1 Stopped"), res.Combined())
	assert.Assert(t, !strings.Contains(res.Combined(), "Container e2e-start-stop-svc-multiple-files-and-another-one-1 Stopped"), res.Combined())
}
