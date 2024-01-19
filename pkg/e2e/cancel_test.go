//go:build !windows
// +build !windows

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
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docker/compose/v2/pkg/utils"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestComposeCancel(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("metrics on cancel Compose build", func(t *testing.T) {
		const buildProjectPath = "fixtures/build-infinite/compose.yaml"

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// require a separate groupID from the process running tests, in order to simulate ctrl+C from a terminal.
		// sending kill signal
		var stdout, stderr utils.SafeBuffer
		cmd, err := StartWithNewGroupID(
			ctx,
			c.NewDockerComposeCmd(t, "-f", buildProjectPath, "build", "--progress", "plain"),
			&stdout,
			&stderr,
		)
		assert.NilError(t, err)
		processDone := make(chan error, 1)
		go func() {
			defer close(processDone)
			processDone <- cmd.Wait()
		}()

		c.WaitForCondition(t, func() (bool, string) {
			out := stdout.String()
			errors := stderr.String()
			return strings.Contains(out,
					"RUN sleep infinity"), fmt.Sprintf("'RUN sleep infinity' not found in : \n%s\nStderr: \n%s\n", out,
					errors)
		}, 30*time.Second, 1*time.Second)

		// simulate Ctrl-C : send signal to processGroup, children will have same groupId by default
		err = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		assert.NilError(t, err)

		select {
		case <-ctx.Done():
			t.Fatal("test context canceled")
		case err := <-processDone:
			// TODO(milas): Compose should really not return exit code 130 here,
			// 	this is an old hack for the compose-cli wrapper
			assert.Error(t, err, "exit status 130",
				"STDOUT:\n%s\nSTDERR:\n%s\n", stdout.String(), stderr.String())
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for Compose exit")
		}
	})
}

func StartWithNewGroupID(ctx context.Context, command icmd.Cmd, stdout *utils.SafeBuffer, stderr *utils.SafeBuffer) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, command.Command[0], command.Command[1:]...)
	cmd.Env = command.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	err := cmd.Start()
	return cmd, err
}
