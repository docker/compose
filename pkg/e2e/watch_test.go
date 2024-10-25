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
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"
)

func TestWatch(t *testing.T) {

	services := []string{"alpine", "busybox", "debian"}
	for _, svcName := range services {
		t.Run(svcName, func(t *testing.T) {
			t.Helper()
			doTest(t, svcName)
		})
	}
}

func TestRebuildOnDotEnvWithExternalNetwork(t *testing.T) {
	const projectName = "test_rebuild_on_dotenv_with_external_network"
	const svcName = "ext-alpine"
	containerName := strings.Join([]string{projectName, svcName, "1"}, "-")
	const networkName = "e2e-watch-external_network_test"
	const dotEnvFilepath = "./fixtures/watch/.env"

	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME="+projectName,
		"COMPOSE_FILE=./fixtures/watch/with-external-network.yaml",
	))

	cleanup := func() {
		c.RunDockerComposeCmdNoCheck(t, "down", "--remove-orphans", "--volumes", "--rmi=local")
		c.RunDockerOrExitError(t, "network", "rm", networkName)
		os.Remove(dotEnvFilepath) //nolint:errcheck
	}
	cleanup()

	t.Log("create network that is referenced by the container we're testing")
	c.RunDockerCmd(t, "network", "create", networkName)
	res := c.RunDockerCmd(t, "network", "ls")
	assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())

	t.Log("create a dotenv file that will be used to trigger the rebuild")
	err := os.WriteFile(dotEnvFilepath, []byte("HELLO=WORLD"), 0o666)
	assert.NilError(t, err)
	_, err = os.ReadFile(dotEnvFilepath)
	assert.NilError(t, err)

	// TODO: refactor this duplicated code into frameworks? Maybe?
	t.Log("starting docker compose watch")
	cmd := c.NewDockerComposeCmd(t, "--verbose", "watch", svcName)
	// stream output since watch runs in the background
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	r := icmd.StartCmd(cmd)
	require.NoError(t, r.Error)
	var testComplete atomic.Bool
	go func() {
		// if the process exits abnormally before the test is done, fail the test
		if err := r.Cmd.Wait(); err != nil && !t.Failed() && !testComplete.Load() {
			assert.Check(t, cmp.Nil(err))
		}
	}()

	t.Log("wait for watch to start watching")
	c.WaitForCondition(t, func() (bool, string) {
		out := r.String()
		errors := r.String()
		return strings.Contains(out,
				"Watch configuration"), fmt.Sprintf("'Watch configuration' not found in : \n%s\nStderr: \n%s\n", out,
				errors)
	}, 30*time.Second, 1*time.Second)

	pn := c.RunDockerCmd(t, "inspect", containerName, "-f", "{{ .HostConfig.NetworkMode }}")
	assert.Equal(t, strings.TrimSpace(pn.Stdout()), networkName)

	t.Log("create a dotenv file that will be used to trigger the rebuild")
	err = os.WriteFile(dotEnvFilepath, []byte("HELLO=WORLD\nTEST=REBUILD"), 0o666)
	assert.NilError(t, err)
	_, err = os.ReadFile(dotEnvFilepath)
	assert.NilError(t, err)

	// NOTE: are there any other ways to check if the container has been rebuilt?
	t.Log("check if the container has been rebuild")
	c.WaitForCondition(t, func() (bool, string) {
		out := r.String()
		if strings.Count(out, "batch complete: service["+svcName+"]") != 1 {
			return false, fmt.Sprintf("container %s was not rebuilt", containerName)
		}
		return true, fmt.Sprintf("container %s was rebuilt", containerName)
	}, 30*time.Second, 1*time.Second)

	pn2 := c.RunDockerCmd(t, "inspect", containerName, "-f", "{{ .HostConfig.NetworkMode }}")
	assert.Equal(t, strings.TrimSpace(pn2.Stdout()), networkName)

	assert.Check(t, !strings.Contains(r.Combined(), "Application failed to start after update"))

	t.Cleanup(cleanup)
	t.Cleanup(func() {
		// IMPORTANT: watch doesn't exit on its own, don't leak processes!
		if r.Cmd.Process != nil {
			t.Logf("Killing watch process: pid[%d]", r.Cmd.Process.Pid)
			_ = r.Cmd.Process.Kill()
		}
	})
	testComplete.Store(true)

}

// NOTE: these tests all share a single Compose file but are safe to run
// concurrently (though that's not recommended).
func doTest(t *testing.T, svcName string) {
	tmpdir := t.TempDir()
	dataDir := filepath.Join(tmpdir, "data")
	configDir := filepath.Join(tmpdir, "config")

	writeTestFile := func(name, contents, sourceDir string) {
		t.Helper()
		dest := filepath.Join(sourceDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o700))
		t.Logf("writing %q to %q", contents, dest)
		require.NoError(t, os.WriteFile(dest, []byte(contents+"\n"), 0o600))
	}
	writeDataFile := func(name, contents string) {
		writeTestFile(name, contents, dataDir)
	}

	composeFilePath := filepath.Join(tmpdir, "compose.yaml")
	CopyFile(t, filepath.Join("fixtures", "watch", "compose.yaml"), composeFilePath)

	projName := "e2e-watch-" + svcName
	env := []string{
		"COMPOSE_FILE=" + composeFilePath,
		"COMPOSE_PROJECT_NAME=" + projName,
	}

	cli := NewCLI(t, WithEnv(env...))

	// important that --rmi is used to prune the images and ensure that watch builds on launch
	cleanup := func() {
		cli.RunDockerComposeCmd(t, "down", svcName, "--remove-orphans", "--volumes", "--rmi=local")
	}
	cleanup()
	t.Cleanup(cleanup)

	cmd := cli.NewDockerComposeCmd(t, "--verbose", "watch", svcName)
	// stream output since watch runs in the background
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	r := icmd.StartCmd(cmd)
	require.NoError(t, r.Error)
	t.Cleanup(func() {
		// IMPORTANT: watch doesn't exit on its own, don't leak processes!
		if r.Cmd.Process != nil {
			t.Logf("Killing watch process: pid[%d]", r.Cmd.Process.Pid)
			_ = r.Cmd.Process.Kill()
		}
	})
	var testComplete atomic.Bool
	go func() {
		// if the process exits abnormally before the test is done, fail the test
		if err := r.Cmd.Wait(); err != nil && !t.Failed() && !testComplete.Load() {
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
			return poll.Continue("%v", res.Combined())
		}
	}

	waitForFlush := func() {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		sentinelVal := fmt.Sprintf("%x", b)
		writeDataFile("wait.txt", sentinelVal)
		poll.WaitOn(t, checkFileContents("/app/data/wait.txt", sentinelVal))
	}

	t.Logf("Writing to a file until Compose watch is up and running")
	poll.WaitOn(t, func(t poll.LogT) poll.Result {
		writeDataFile("hello.txt", "hello world")
		return checkFileContents("/app/data/hello.txt", "hello world")(t)
	}, poll.WithDelay(time.Second))

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
		})

	t.Logf("Writing to ignored paths")
	writeDataFile("data.foo", "ignored")
	writeDataFile(filepath.Join("ignored", "hello.txt"), "ignored")
	waitForFlush()
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/data.foo").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		})
	cli.RunDockerComposeCmdNoCheck(t, "exec", svcName, "stat", "/app/data/ignored").
		Assert(t, icmd.Expected{
			ExitCode: 1,
			Err:      "No such file or directory",
		})

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
		})

	t.Logf("Sync and restart use case")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	writeTestFile("file.config", "This is an updated config file", configDir)
	checkRestart := func(state string) poll.Check {
		return func(pollLog poll.LogT) poll.Result {
			if strings.Contains(r.Combined(), state) {
				return poll.Success()
			}
			return poll.Continue("%v", r.Combined())
		}
	}
	poll.WaitOn(t, checkRestart(fmt.Sprintf("service %q restarted", svcName)))
	poll.WaitOn(t, checkFileContents("/app/config/file.config", "This is an updated config file"))

	testComplete.Store(true)
}
