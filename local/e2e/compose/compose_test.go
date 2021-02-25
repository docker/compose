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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/utils/e2e"
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

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/sentences/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check accessing running app", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:90"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd("network", "ls")
		res.Assert(t, icmd.Expected{Out: projectName + "_default"})
	})

	t.Run("check compose labels", func(t *testing.T) {
		wd, _ := os.Getwd()
		res := c.RunDockerCmd("inspect", projectName+"_web_1")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "compose-e2e-demo"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "False",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.config-hash":`})
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf(`"com.docker.compose.project.config_files": "%s/fixtures/sentences/compose.yaml"`, wd)})
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

	t.Run("check healthcheck output", func(t *testing.T) {
		c.WaitForCmdResult(c.NewDockerCmd("compose", "-p", projectName, "ps", "--format", "json"),
			StdoutContains(`"Name":"compose-e2e-demo_web_1","Project":"compose-e2e-demo","Service":"web","State":"running","Health":"healthy"`),
			5*time.Second, 1*time.Second)

		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `NAME                       SERVICE             STATUS              PORTS`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo_web_1     web                 running (healthy)   0.0.0.0:90->80/tcp`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo_db_1      db                  running             5432/tcp`})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
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

func TestComposePull(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	res := c.RunDockerOrExitError("compose", "--project-directory", "fixtures/simple-composefile", "pull")
	output := res.Combined()

	assert.Assert(t, strings.Contains(output, "simple Pulled"))
	assert.Assert(t, strings.Contains(output, "another Pulled"))
}
