//go:build !windows

/*
   Copyright 2022 Docker Compose CLI authors

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
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/utils"
)

func TestUpServiceUnhealthy(t *testing.T) {
	s := NewScenario(t, "up must fail when a service never turns healthy")
	s.Step("up reports the unhealthy container and fails",
		ComposeCmd("up", "-d").MayFail().Within(60*time.Second),
		ExitCode(1),
		OutputContains("container "+s.Project()+"-fail-1 is unhealthy"),
		ServiceState("depends", "created"))
}

func TestUpDependenciesNotStopped(t *testing.T) {
	c := NewParallelCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME=up-deps-stop",
	))

	reset := func() {
		c.RunDockerComposeCmdNoCheck(t, "down", "-t=0", "--remove-orphans", "-v")
	}
	reset()
	t.Cleanup(reset)

	t.Log("Launching orphan container (background)")
	c.RunDockerComposeCmd(t,
		"-f=./fixtures/ups-deps-stop/orphan.yaml",
		"up",
		"--wait",
		"--detach",
		"orphan",
	)
	RequireServiceState(t, c, "orphan", "running")

	t.Log("Launching app container with implicit dependency")
	upOut := &utils.SafeBuffer{}
	testCmd := c.NewDockerComposeCmd(t,
		"-f=./fixtures/ups-deps-stop/compose.yaml",
		"up",
		"--menu=false",
		"app",
	)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)

	cmd, err := StartWithNewGroupID(ctx, testCmd, upOut, nil)
	assert.NilError(t, err, "Failed to run compose up")

	t.Log("Waiting for containers to be in running state")
	upOut.RequireEventuallyContains(t, "hello app")
	RequireServiceState(t, c, "app", "running")
	RequireServiceState(t, c, "dependency", "running")

	t.Log("Simulating Ctrl-C")
	assert.NilError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT),
		"Failed to send SIGINT to compose up process")

	t.Log("Waiting for `compose up` to exit")
	err = cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		if exitErr.ExitCode() == -1 {
			t.Fatalf("`compose up` was killed: %v", err)
		}
		assert.Equal(t, 130, exitErr.ExitCode())
	}

	RequireServiceState(t, c, "app", "exited")
	// dependency should still be running
	RequireServiceState(t, c, "dependency", "running")
	RequireServiceState(t, c, "orphan", "running")
}

func TestUpWithBuildDependencies(t *testing.T) {
	s := NewScenario(t, "up must build a service's image before starting another service that reuses it")
	image := s.Project() + "-built"
	s.Env("BUILT_IMAGE="+image).
		Defer(DockerCmd("image", "rm", "-f", image).MayFail()).
		Step("up builds once and starts both services from the built image",
			ComposeCmd("up", "-d"),
			ImageExists(image))
}

func TestUpWithDependencyExit(t *testing.T) {
	s := NewScenario(t, "up must fail when a dependency exits before turning healthy")
	s.Step("up reports the exited dependency and fails",
		ComposeCmd("up", "-d").MayFail(),
		ExitCode(1),
		OutputContains("dependency failed to start: container "+s.Project()+"-db-1 exited (1)"),
		ServiceState("web", "created"))
}

func TestScaleDoesntRecreate(t *testing.T) {
	NewScenario(t, "scaling up must add a replica without recreating the existing one").
		Step("up starts the first replica",
			ComposeCmd("up", "-d"),
			ReplicaNumbers("simple", 1)).
		Step("up --scale adds the second replica, keeping the first",
			ComposeCmd("up", "--scale", "simple=2", "-d"),
			ReplicaNumbers("simple", 1, 2),
			OutputNotContains("Recreated"))
}

func TestUpWithDependencyNotRequired(t *testing.T) {
	NewScenario(t, "up must start the service even when an optional dependency cannot").
		Step("up succeeds, reporting the optional dependency failure",
			ComposeCmd("--profile", "not-required", "up", "-d"),
			OutputContains("foo"),
			OutputContains(`optional dependency "bar" failed to start`))
}

func TestUpWithAllResources(t *testing.T) {
	s := NewScenario(t, "up --all-resources must create volumes and networks no service uses")
	s.Step("up creates the unused volume and network",
		ComposeCmd("--all-resources", "up"),
		OutputContains("Volume "+s.Project()+"_my_vol Created"),
		OutputContains("Network "+s.Project()+"_my_net Created"))
}

func TestUpProfile(t *testing.T) {
	NewScenario(t, "up on a profiled service must start it and its dependencies, not its profile siblings").
		Step("up starts the target and its dependency only",
			ComposeCmd("up", "foo"),
			ServiceState("foo", "exited"),
			ServiceState("db", "exited"),
			ServiceNotCreated("bar"))
}

func TestUpImageID(t *testing.T) {
	s := NewScenario(t, "a service image referenced by its bare ID must be usable")
	digest := strings.TrimSpace(s.CLI().RunDockerCmd(t, "image", "inspect", "alpine", "-f", "{{ .ID }}").Stdout())
	_, id, _ := strings.Cut(digest, ":")
	s.Env("ID="+id).
		Step("up runs the container from the image ID",
			ComposeCmd("up"))
}

func TestUpStopWithLogsMixed(t *testing.T) {
	s := NewScenario(t, "on abort, logs of surviving services must keep flowing while others stop, hooks included")
	s.Step("up aborts on the first exit but still relays service2's logs and stop hook",
		ComposeCmd("up", "--abort-on-container-exit").Within(60*time.Second),
		StderrContains("Container "+s.Project()+"-service1-1 Stopped"),
		StdoutContains("stop hook running..."),
		StdoutContains("64 bytes"))
}
