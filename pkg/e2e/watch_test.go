/*
   Copyright 2023 Docker Compose CLI authors

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distribution/distribution/v3/uuid"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestWatch(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Test currently broken on macOS due to symlink issues (see compose-go#436)")
	}

	services := []string{"alpine", "busybox", "debian"}
	for _, svcName := range services {
		t.Run(svcName, func(t *testing.T) {
			t.Helper()
			doTest(t, svcName)
		})
	}
}

// NOTE: these tests all share a single Compose file but are safe to run concurrently
func doTest(t *testing.T, svcName string) {
	tmpdir := t.TempDir()
	dataDir := filepath.Join(tmpdir, "data")
	writeDataFile := func(name string, contents string) {
		t.Helper()
		dest := filepath.Join(dataDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o700))
		t.Logf("writing %q to %q", contents, dest)
		require.NoError(t, os.WriteFile(dest, []byte(contents+"\n"), 0o600))
	}

	composeFilePath := filepath.Join(tmpdir, "compose.yaml")
	CopyFile(t, filepath.Join("fixtures", "watch", "compose.yaml"), composeFilePath)

	projName := "e2e-watch-" + svcName
	env := []string{
		"COMPOSE_FILE=" + composeFilePath,
		"COMPOSE_PROJECT_NAME=" + projName,
	}

	cli := NewCLI(t, WithEnv(env...))

	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", svcName, "--timeout=0", "--remove-orphans", "--volumes")
	}
	cleanup()
	t.Cleanup(cleanup)

	cli.RunDockerComposeCmd(t, "up", svcName, "--wait", "--build")

	cmd := cli.NewDockerComposeCmd(t, "--verbose", "alpha", "watch", svcName)
	// stream output since watch runs in the background
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	r := icmd.StartCmd(cmd)
	require.NoError(t, r.Error)
	t.Cleanup(func() {
		// IMPORTANT: watch doesn't exit on its own, don't leak processes!
		if r.Cmd.Process != nil {
			_ = r.Cmd.Process.Kill()
		}
	})
	var testComplete atomic.Bool
	go func() {
		// if the process exits abnormally before the test is done, fail the test
		if err := r.Cmd.Wait(); err != nil && !testComplete.Load() {
			assert.Check(t, cmp.Nil(err))
		}
	}()

	require.NoError(t, os.Mkdir(dataDir, 0o700))

	checkFileContents := func(path string, contents string) poll.Check {
		return func(pollLog poll.LogT) poll.Result {
			if r.Cmd.ProcessState != nil {
				return poll.Error(fmt.Errorf("watch process exited early: %s", r.Cmd.ProcessState))
			}
			res := icmd.RunCmd(cli.NewDockerComposeCmd(t, "exec", svcName, "cat", path))
			if strings.Contains(res.Stdout(), contents) {
				return poll.Success()
			}
			return poll.Continue(res.Combined())
		}
	}

	waitForFlush := func() {
		sentinelVal := uuid.Generate().String()
		writeDataFile("wait.txt", sentinelVal)
		poll.WaitOn(t, checkFileContents("/app/data/wait.txt", sentinelVal))
	}

	t.Logf("Writing to a file until Compose watch is up and running")
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		writeDataFile("hello.txt", "hello world")
		return checkFileContents("/app/data/hello.txt", "hello world")(t)
	})

	t.Logf("Modifying file contents")
	writeDataFile("hello.txt", "hello watch")
	poll.WaitOn(t, checkFileContents("/app/data/hello.txt", "hello watch"))

	t.Logf("Deleting file")
	require.NoError(t, os.Remove(filepath.Join(dataDir, "hello.txt")))
	waitForFlush()
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/hello.txt").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		},
		)

	t.Logf("Writing to ignored paths")
	writeDataFile("data.foo", "ignored")
	writeDataFile(filepath.Join("ignored", "hello.txt"), "ignored")
	waitForFlush()
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/data.foo").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		},
		)
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/ignored").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		},
		)

	t.Logf("Creating subdirectory")
	require.NoError(t, os.Mkdir(filepath.Join(dataDir, "subdir"), 0o700))
	waitForFlush()
	cli.RunDockerComposeCmd(t, "exec", svcName, "stat", "/app/data/subdir")

	t.Logf("Writing to file in subdirectory")
	writeDataFile(filepath.Join("subdir", "file.txt"), "a")
	poll.WaitOn(t, checkFileContents("/app/data/subdir/file.txt", "a"))

	t.Logf("Writing to file multiple times")
	writeDataFile(filepath.Join("subdir", "file.txt"), "x")
	writeDataFile(filepath.Join("subdir", "file.txt"), "y")
	writeDataFile(filepath.Join("subdir", "file.txt"), "z")
	poll.WaitOn(t, checkFileContents("/app/data/subdir/file.txt", "z"))
	writeDataFile(filepath.Join("subdir", "file.txt"), "z")
	writeDataFile(filepath.Join("subdir", "file.txt"), "y")
	writeDataFile(filepath.Join("subdir", "file.txt"), "x")
	poll.WaitOn(t, checkFileContents("/app/data/subdir/file.txt", "x"))

	t.Logf("Deleting directory")
	require.NoError(t, os.RemoveAll(filepath.Join(dataDir, "subdir")))
	waitForFlush()
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/subdir").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		},
		)

	testComplete.Store(true)
}
