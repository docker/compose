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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/poll"

	. "github.com/docker/compose-cli/utils/e2e"
)

func TestKillChildProcess(t *testing.T) {
	assert.Assert(t, runtime.GOOS != "windows", "cannot test process signals on windows")
	c := NewParallelE2eCLI(t, binDir)

	image := "test-sleep-image"
	psCmd := icmd.Command("ps", "-x")
	psRes := icmd.RunCmd(psCmd)
	psRes.Assert(t, icmd.Success)
	assert.Assert(t, !strings.Contains(psRes.Combined(), image))

	d := writeDockerfile(t)
	buildArgs := []string{"build", "--no-cache", "-t", image, "."}
	cmd := c.NewDockerCmd(buildArgs...)
	cmd.Dir = d
	res := icmd.StartCmd(cmd)

	buildRunning := func(t poll.LogT) poll.Result {
		res := icmd.RunCmd(psCmd)
		if strings.Contains(res.Combined(), strings.Join(buildArgs, " ")) {
			return poll.Success()
		}
		return poll.Continue("waiting for child process to be running")
	}
	poll.WaitOn(t, buildRunning, poll.WithDelay(1*time.Second))

	err := res.Cmd.Process.Signal(syscall.SIGTERM)
	assert.NilError(t, err)
	buildStopped := func(t poll.LogT) poll.Result {
		res := icmd.RunCmd(psCmd)
		if !strings.Contains(res.Combined(), strings.Join(buildArgs, " ")) {
			return poll.Success()
		}
		return poll.Continue("waiting for child process to be killed")
	}
	poll.WaitOn(t, buildStopped, poll.WithDelay(1*time.Second), poll.WithTimeout(60*time.Second))
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
