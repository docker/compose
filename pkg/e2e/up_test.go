//go:build !windows
// +build !windows

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
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/icmd"
)

func TestUpServiceUnhealthy(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-fail"

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/start-fail/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: `container for service "fail" is unhealthy`})

	c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	var upOut lockedBuffer
	var upCmd *exec.Cmd
	go func() {
		testCmd := c.NewDockerComposeCmd(t,
			"-f=./fixtures/ups-deps-stop/compose.yaml",
			"up",
			"app",
		)
		cmd := exec.CommandContext(ctx, testCmd.Command[0], testCmd.Command[1:]...)
		cmd.Env = testCmd.Env
		cmd.Stdout = &upOut
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		assert.NoError(t, cmd.Start(), "Failed to run compose up")
		upCmd = cmd
	}()

	t.Log("Waiting for containers to be in running state")
	upOut.RequireEventuallyContains(t, "hello app")
	RequireServiceState(t, c, "app", "running")
	RequireServiceState(t, c, "dependency", "running")

	t.Log("Simulating Ctrl-C")
	require.NoError(t, syscall.Kill(-upCmd.Process.Pid, syscall.SIGINT),
		"Failed to send SIGINT to compose up process")

	time.AfterFunc(5*time.Second, cancel)

	t.Log("Waiting for `compose up` to exit")
	err := upCmd.Wait()
	if err != nil {
		exitErr := err.(*exec.ExitError)
		require.EqualValues(t, exitErr.ExitCode(), 130)
	}

	RequireServiceState(t, c, "app", "exited")
	// dependency should still be running
	RequireServiceState(t, c, "dependency", "running")
	RequireServiceState(t, c, "orphan", "running")
}
