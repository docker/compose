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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/poll"

	"gotest.tools/v3/icmd"
)

func TestLocalComposeLogs(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-logs"

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/logs-test/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("logs", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "logs")
		res.Assert(t, icmd.Expected{Out: `PING localhost`})
		res.Assert(t, icmd.Expected{Out: `hello`})
	})

	t.Run("logs ping", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "logs", "ping")
		res.Assert(t, icmd.Expected{Out: `PING localhost`})
		assert.Assert(t, !strings.Contains(res.Stdout(), "hello"))
	})

	t.Run("logs hello", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "logs", "hello", "ping")
		res.Assert(t, icmd.Expected{Out: `PING localhost`})
		res.Assert(t, icmd.Expected{Out: `hello`})
	})

	t.Run("logs hello index", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "logs", "--index", "2", "hello")

		//  docker-compose logs hello
		// logs-test-hello-2  | hello
		// logs-test-hello-1  | hello
		t.Log(res.Stdout())
		assert.Assert(t, !strings.Contains(res.Stdout(), "hello-1"))
		assert.Assert(t, strings.Contains(res.Stdout(), "hello-2"))
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestLocalComposeLogsFollow(t *testing.T) {
	c := NewCLI(t, WithEnv("REPEAT=20"))
	const projectName = "compose-e2e-logs"
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	c.RunDockerComposeCmd(t, "-f", "./fixtures/logs-test/compose.yaml", "--project-name", projectName, "up", "-d", "ping")

	cmd := c.NewDockerComposeCmd(t, "--project-name", projectName, "logs", "-f")
	res := icmd.StartCmd(cmd)
	t.Cleanup(func() {
		_ = res.Cmd.Process.Kill()
	})

	poll.WaitOn(t, expectOutput(res, "ping-1 "), poll.WithDelay(100*time.Millisecond), poll.WithTimeout(1*time.Second))

	c.RunDockerComposeCmd(t, "-f", "./fixtures/logs-test/compose.yaml", "--project-name", projectName, "up", "-d")

	poll.WaitOn(t, expectOutput(res, "hello-1 "), poll.WithDelay(100*time.Millisecond), poll.WithTimeout(1*time.Second))

	c.RunDockerComposeCmd(t, "-f", "./fixtures/logs-test/compose.yaml", "--project-name", projectName, "up", "-d", "--scale", "ping=2", "ping")

	poll.WaitOn(t, expectOutput(res, "ping-2 "), poll.WithDelay(100*time.Millisecond), poll.WithTimeout(20*time.Second))
}

func TestLocalComposeLargeLogs(t *testing.T) {
	const projectName = "compose-e2e-large_logs"
	file := filepath.Join(t.TempDir(), "large.txt")
	c := NewCLI(t, WithEnv("FILE="+file))
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	f, err := os.Create(file)
	assert.NilError(t, err)
	for i := 0; i < 300_000; i++ {
		_, err := io.WriteString(f, fmt.Sprintf("This is line %d in a laaaarge text file\n", i))
		assert.NilError(t, err)
	}
	assert.NilError(t, f.Close())

	cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/logs-test/cat.yaml", "--project-name", projectName, "up", "--abort-on-container-exit", "--menu=false")
	cmd.Stdout = io.Discard
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{Out: "test-1 exited with code 0"})
}

func expectOutput(res *icmd.Result, expected string) func(t poll.LogT) poll.Result {
	return func(t poll.LogT) poll.Result {
		if strings.Contains(res.Stdout(), expected) {
			return poll.Success()
		}
		return poll.Continue("condition not met")

	}
}
