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
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docker/compose/v2/pkg/utils"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestUpServiceUnhealthy(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-start-fail"

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "fixtures/start-fail/compose.yaml", "--project-name", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{ExitCode: 1, Err: `container e2e-start-fail-fail-1 is unhealthy`})

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	cmd, err := StartWithNewGroupID(ctx, testCmd, upOut, nil)
	assert.NilError(t, err, "Failed to run compose up")

	t.Log("Waiting for containers to be in running state")
	upOut.RequireEventuallyContains(t, "hello app")
	RequireServiceState(t, c, "app", "running")
	RequireServiceState(t, c, "dependency", "running")

	t.Log("Simulating Ctrl-C")
	require.NoError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT),
		"Failed to send SIGINT to compose up process")

	t.Log("Waiting for `compose up` to exit")
	err = cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		if exitErr.ExitCode() == -1 {
			t.Fatalf("`compose up` was killed: %v", err)
		}
		require.EqualValues(t, exitErr.ExitCode(), 130)
	}

	RequireServiceState(t, c, "app", "exited")
	// dependency should still be running
	RequireServiceState(t, c, "dependency", "running")
	RequireServiceState(t, c, "orphan", "running")
}

func TestUpWithBuildDependencies(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("up with service using image build by an another service", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "built-image-dependency")

		res := c.RunDockerComposeCmd(t, "--project-directory", "fixtures/dependencies",
			"-f", "fixtures/dependencies/service-image-depends-on.yaml", "up", "-d")

		t.Cleanup(func() {
			c.RunDockerComposeCmd(t, "--project-directory", "fixtures/dependencies",
				"-f", "fixtures/dependencies/service-image-depends-on.yaml", "down", "--rmi", "all")
		})

		res.Assert(t, icmd.Success)
	})
}

func TestUpWithDependencyExit(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("up with dependency to exit before being healthy", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "--project-directory", "fixtures/dependencies",
			"-f", "fixtures/dependencies/dependency-exit.yaml", "up", "-d")

		t.Cleanup(func() {
			c.RunDockerComposeCmd(t, "--project-name", "dependencies", "down")
		})

		res.Assert(t, icmd.Expected{ExitCode: 1, Err: "dependency failed to start: container dependencies-db-1 exited (1)"})
	})
}

func TestScaleDoesntRecreate(t *testing.T) {
	c := NewCLI(t)
	const projectName = "compose-e2e-scale"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	c.RunDockerComposeCmd(t, "-f", "fixtures/simple-composefile/compose.yaml", "--project-name", projectName, "up", "-d")

	res := c.RunDockerComposeCmd(t, "-f", "fixtures/simple-composefile/compose.yaml", "--project-name", projectName, "up", "--scale", "simple=2", "-d")
	assert.Check(t, !strings.Contains(res.Combined(), "Recreated"))

}

func TestUpWithDependencyNotRequired(t *testing.T) {
	c := NewCLI(t)
	const projectName = "compose-e2e-dependency-not-required"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/deps-not-required.yaml", "--project-name", projectName,
		"--profile", "not-required", "up", "-d")
	assert.Assert(t, strings.Contains(res.Combined(), "foo"), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), " optional dependency \"bar\" failed to start"), res.Combined())
}

func TestUpWithAllResources(t *testing.T) {
	c := NewCLI(t)
	const projectName = "compose-e2e-all-resources"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "-v")
	})

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/resources/compose.yaml", "--all-resources", "--project-name", projectName, "up")
	assert.Assert(t, strings.Contains(res.Combined(), fmt.Sprintf(`Volume "%s_my_vol"  Created`, projectName)), res.Combined())
	assert.Assert(t, strings.Contains(res.Combined(), fmt.Sprintf(`Network %s_my_net  Created`, projectName)), res.Combined())
}
