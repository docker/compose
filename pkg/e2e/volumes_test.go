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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeVolume(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-volume"

	t.Run("up with build and no image name, volume", func(t *testing.T) {
		// ensure local test run does not reuse previously build image
		c.RunDockerOrExitError(t, "rmi", "compose-e2e-volume-nginx")
		c.RunDockerOrExitError(t, "volume", "rm", projectName+"-staticVol")
		c.RunDockerOrExitError(t, "volume", "rm", "myvolume")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up",
			"-d")
	})

	t.Run("access bind mount data", func(t *testing.T) {
		output := HTTPGetWithRetry(t, "http://localhost:8090", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))
	})

	t.Run("check container volume specs", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", "compose-e2e-volume-nginx2-1", "--format", "{{ json .Mounts }}")
		output := res.Stdout()
		assert.Assert(t, strings.Contains(output, `"Destination":"/usr/src/app/node_modules","Driver":"local","Mode":"z","RW":true,"Propagation":""`), output)
		assert.Assert(t, strings.Contains(output, `"Destination":"/myconfig","Mode":"","RW":false,"Propagation":"rprivate"`), output)
	})

	t.Run("check config content", func(t *testing.T) {
		output := c.RunDockerCmd(t, "exec", "compose-e2e-volume-nginx2-1", "cat", "/myconfig").Stdout()
		assert.Assert(t, strings.Contains(output, `Hello from Nginx container`), output)
	})

	t.Run("check secrets content", func(t *testing.T) {
		output := c.RunDockerCmd(t, "exec", "compose-e2e-volume-nginx2-1", "cat", "/run/secrets/mysecret").Stdout()
		assert.Assert(t, strings.Contains(output, `Hello from Nginx container`), output)
	})

	t.Run("check container bind-mounts specs", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", "compose-e2e-volume-nginx-1", "--format", "{{ json .Mounts }}")
		output := res.Stdout()
		assert.Assert(t, strings.Contains(output, `"Type":"bind"`))
		assert.Assert(t, strings.Contains(output, `"Destination":"/usr/share/nginx/html"`))
	})

	t.Run("should inherit anonymous volumes", func(t *testing.T) {
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "touch", "/usr/src/app/node_modules/test")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up", "--force-recreate", "-d")
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "ls", "/usr/src/app/node_modules/test")
	})

	t.Run("should renew anonymous volumes", func(t *testing.T) {
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "touch", "/usr/src/app/node_modules/test")
		c.RunDockerComposeCmd(t, "--project-directory", "fixtures/volume-test", "--project-name", projectName, "up", "--force-recreate", "--renew-anon-volumes", "-d")
		c.RunDockerOrExitError(t, "exec", "compose-e2e-volume-nginx2-1", "ls", "/usr/src/app/node_modules/test")
	})

	t.Run("cleanup volume project", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "--volumes")
		ls := c.RunDockerCmd(t, "volume", "ls").Stdout()
		assert.Assert(t, !strings.Contains(ls, projectName+"-staticVol"))
		assert.Assert(t, !strings.Contains(ls, "myvolume"))
	})
}

func TestProjectVolumeBind(t *testing.T) {
	if composeStandaloneMode {
		t.Skip()
	}
	c := NewParallelCLI(t)
	const projectName = "compose-e2e-project-volume-bind"

	t.Run("up on project volume with bind specification", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Running on Windows. Skipping...")
		}
		tmpDir, err := os.MkdirTemp("", projectName)
		assert.NilError(t, err)
		defer os.RemoveAll(tmpDir) //nolint

		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		c.RunDockerOrExitError(t, "volume", "rm", "-f", projectName+"_project-data").Assert(t, icmd.Success)
		cmd := c.NewCmdWithEnv([]string{"TEST_DIR=" + tmpDir},
			"docker", "compose", "--project-directory", "fixtures/project-volume-bind-test", "--project-name", projectName, "up", "-d")
		icmd.RunCmd(cmd).Assert(t, icmd.Success)
		defer c.RunDockerComposeCmd(t, "--project-name", projectName, "down")

		c.RunCmd(t, "sh", "-c", "echo SUCCESS > "+filepath.Join(tmpDir, "resultfile")).Assert(t, icmd.Success)

		ret := c.RunDockerOrExitError(t, "exec", "frontend", "bash", "-c", "cat /data/resultfile").Assert(t, icmd.Success)
		assert.Assert(t, strings.Contains(ret.Stdout(), "SUCCESS"))
	})
}
