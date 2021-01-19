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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/golden"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	"github.com/docker/compose-cli/cli/cmd"
	. "github.com/docker/compose-cli/tests/framework"
)

var binDir string

func TestMain(m *testing.M) {
	p, cleanup, err := SetupExistingCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	binDir = p
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestKillChildProcess(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	image := "test-sleep-image"
	pCmd := icmd.Command("ps", "-x")
	if runtime.GOOS == "windows" {
		pCmd = icmd.Command("tasklist")
	}
	pRes := icmd.RunCmd(pCmd)
	pRes.Assert(t, icmd.Success)
	assert.Assert(t, !strings.Contains(pRes.Combined(), image))

	d := writeDockerfile(t)
	buildArgs := []string{"build", "--no-cache", "-t", image, "."}
	cmd := c.NewDockerCmd(buildArgs...)
	cmd.Dir = d
	res := icmd.StartCmd(cmd)

	buildRunning := func(t poll.LogT) poll.Result {
		res := icmd.RunCmd(pCmd)
		if strings.Contains(res.Combined(), strings.Join(buildArgs, " ")) {
			return poll.Success()
		}
		return poll.Continue("waiting for child process to be running")
	}
	poll.WaitOn(t, buildRunning, poll.WithDelay(1*time.Second))

	if runtime.GOOS == "windows" {
		err := res.Cmd.Process.Kill()
		assert.NilError(t, err)
	} else {
		err := res.Cmd.Process.Signal(syscall.SIGTERM)
		assert.NilError(t, err)
	}
	buildStopped := func(t poll.LogT) poll.Result {
		res := icmd.RunCmd(pCmd)
		if !strings.Contains(res.Combined(), strings.Join(buildArgs, " ")) {
			return poll.Success()
		}
		return poll.Continue("waiting for child process to be killed")
	}
	poll.WaitOn(t, buildStopped, poll.WithDelay(1*time.Second), poll.WithTimeout(60*time.Second))
}

// no linux containers on GHA Windows CI nodes (windows server)
func TestLocalContainers(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	c.RunDockerCmd("context", "create", "local", "test-local")
	res := c.RunDockerCmd("context", "use", "test-local")
	res.Assert(t, icmd.Expected{Out: "test-local"})

	t.Run("use", func(t *testing.T) {
		res := c.RunDockerCmd("context", "show")
		res.Assert(t, icmd.Expected{Out: "test-local"})
		res = c.RunDockerCmd("context", "ls")
		golden.Assert(t, res.Stdout(), GoldenFile("ls-out-test-local"))
	})

	var nginxContainerName string
	t.Run("run", func(t *testing.T) {
		res := c.RunDockerCmd("run", "-d", "-p", "85:80", "nginx")
		nginxContainerName = strings.TrimSpace(res.Stdout())
	})
	defer c.RunDockerOrExitError("rm", "-f", nginxContainerName)

	var nginxID string
	t.Run("inspect", func(t *testing.T) {
		res = c.RunDockerCmd("inspect", nginxContainerName)

		inspect := &cmd.ContainerInspectView{}
		err := json.Unmarshal([]byte(res.Stdout()), inspect)
		assert.NilError(t, err)
		nginxID = inspect.ID
	})

	t.Run("ps", func(t *testing.T) {
		res = c.RunDockerCmd("ps")
		lines := Lines(res.Stdout())
		nginxFound := false
		for _, line := range lines {
			fields := strings.Fields(line)
			if fields[0] == nginxID {
				nginxFound = true
				assert.Equal(t, fields[1], "nginx")
				assert.Equal(t, fields[2], "/docker-entrypoint.sh")
			}
		}
		assert.Assert(t, nginxFound, res.Stdout())

		res = c.RunDockerCmd("ps", "--format", "json")
		res.Assert(t, icmd.Expected{Out: `"Image":"nginx","Status":"Up Less than a second","Command":"/docker-entrypoint.sh nginx -g 'daemon off;'","Ports":["0.0.0.0:85->80/tcp"`})

		res = c.RunDockerCmd("ps", "--quiet")
		res.Assert(t, icmd.Expected{Out: nginxID + "\n"})
	})
}

func writeDockerfile(t *testing.T) string {
	d, err := ioutil.TempDir("", "")
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(d)
	})
	err = ioutil.WriteFile(filepath.Join(d, "Dockerfile"), []byte(`FROM alpine:3.10
RUN sleep 100`), 0644)
	assert.NilError(t, err)
	return d
}
