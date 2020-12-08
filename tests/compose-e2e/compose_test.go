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
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	"gotest.tools/v3/icmd"

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

func TestLocalComposeUp(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-demo"

	t.Run("build", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "build", "-f", "../composefiles/demo_multi_port.yaml")
		res.Assert(t, icmd.Expected{Out: "COPY words.sql /docker-entrypoint-initdb.d/"})
		res.Assert(t, icmd.Expected{Out: "COPY pom.xml ."})
		res.Assert(t, icmd.Expected{Out: "COPY static /static/"})
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "up", "-d", "-f", "../composefiles/demo_multi_port.yaml", "--project-name", projectName, "-d")
	})

	t.Run("check running project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "-p", projectName)
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:80"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd("network", "ls")
		res.Assert(t, icmd.Expected{Out: projectName + "_default"})
	})

	t.Run("check compose labels", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"_web_1")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "compose-e2e-demo"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "False",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.config-hash":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.config_files": "../composefiles/demo_multi_port.yaml"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.working_dir":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.service": "web"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version":`})

		res = c.RunDockerCmd("network", "inspect", projectName+"_default")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.network": "default"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": `})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version": `})
	})

	t.Run("check user labels", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"_web_1")
		res.Assert(t, icmd.Expected{Out: `"my-label": "test"`})

	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "down", "--project-name", projectName)
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})

	t.Run("check networks after down", func(t *testing.T) {
		res := c.RunDockerCmd("network", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestLocalComposeVolume(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-volume"

	t.Run("up with build and no image name, volume", func(t *testing.T) {
		//ensure local test run does not reuse previously build image
		c.RunDockerOrExitError("rmi", "compose-e2e-volume_nginx")
		c.RunDockerOrExitError("volume", "rm", projectName+"_staticVol")
		c.RunDockerCmd("compose", "up", "-d", "--workdir", "volume-test", "--project-name", projectName)

		output := HTTPGetWithRetry(t, "http://localhost:8090", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))

		_ = c.RunDockerCmd("compose", "down", "--project-name", projectName)
		_ = c.RunDockerCmd("volume", "rm", projectName+"_staticVol")
	})
}
