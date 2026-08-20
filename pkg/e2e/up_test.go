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
	"strconv"
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
	// service2 pings forever so the abort always interrupts it: with a bounded
	// ping, on a fast machine it can exit on its own before the abort reaches
	// it, and the pre_stop hook never runs.
	s := NewScenario(t, "on abort, logs of surviving services must keep flowing while others stop, hooks included")
	s.Step("up aborts on the first exit but still relays service2's logs and stop hook",
		ComposeCmd("up", "--abort-on-container-exit").Within(60*time.Second),
		StderrContains("Container "+s.Project()+"-service1-1 Stopped"),
		StdoutContains("stop hook running..."),
		StdoutContains("64 bytes"))
}

func TestUpExitsOnExternalStopOfRestartingContainer(t *testing.T) {
	// Regression test for https://github.com/docker/compose/issues/13985
	// A container stopped while in restart backoff emits no `die` event, only
	// `stop`, so an attached `up` waiting for `die` hangs forever. Up to
	// v2.39.2 the attached `up` exited a few seconds after an external stop:
	// containers only need to be stopped, not removed (no `destroy` event).
	c := NewParallelCLI(t)
	const projectName = "e2e-restart-backoff-stop"

	up := startRestartBackoffProject(t, c, projectName)

	c.RunDockerComposeCmd(t, "--project-name", projectName, "stop", "-t", "5")

	assertUpExits(t, up, "external stop")
}

func TestUpExitsOnExternalDownOfRestartingContainer(t *testing.T) {
	// `down` variant of TestUpExitsOnExternalStopOfRestartingContainer: the
	// container in restart backoff emits `stop` then `destroy`, still no `die`.
	c := NewParallelCLI(t)
	const projectName = "e2e-restart-backoff-down"

	up := startRestartBackoffProject(t, c, projectName)

	c.RunDockerComposeCmd(t, "--project-name", projectName, "stop", "-t", "5")
	c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-t", "5")

	assertUpExits(t, up, "external stop+down")
}

// startRestartBackoffProject starts an attached `up` on the restart-backoff
// fixture and waits until the crasher container is in restart backoff, so an
// external stop reliably lands while no process is running (no `die` emitted).
func startRestartBackoffProject(t *testing.T, c *CLI, projectName string) *icmd.Result {
	t.Helper()
	cleanup := func() {
		c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--timeout=0", "--remove-orphans")
	}
	t.Cleanup(cleanup)
	cleanup()

	cmd := c.NewDockerComposeCmd(t, "--ansi=never", "-f", "./fixtures/restart-backoff/compose.yaml",
		"--project-name", projectName, "up", "--menu=false")
	up := icmd.StartCmd(cmd)
	assert.NilError(t, up.Error)
	t.Cleanup(func() {
		if up.Cmd.Process != nil {
			_ = up.Cmd.Process.Kill()
		}
	})

	// wait for enough crash/restart cycles that the backoff window is several
	// seconds wide, so the external stop below reliably lands while the
	// container is in restart backoff
	c.WaitForCondition(t, func() (bool, string) {
		res := c.RunDockerOrExitError(t, "inspect", "--format", "{{.State.Status}} {{.RestartCount}}",
			projectName+"-crasher-1")
		out := strings.TrimSpace(res.Stdout())
		status, count, _ := strings.Cut(out, " ")
		restarts, err := strconv.Atoi(count)
		return err == nil && status == "restarting" && restarts >= 5, fmt.Sprintf("crasher not in restart backoff yet: %q", out)
	}, 2*time.Minute, 500*time.Millisecond)
	return up
}

func assertUpExits(t *testing.T, up *icmd.Result, scenario string) {
	t.Helper()
	finished := make(chan struct{})
	go func() {
		_ = up.Cmd.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		// attached up detected project termination and exited
	case <-time.After(30 * time.Second):
		t.Fatalf("attached up did not exit after %s:\n%s", scenario, up.Combined())
	}
}
