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
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/tests/framework"
)

func TestLocalComposeUp(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	c.RunDockerCmd("context", "create", "local", "test-context").Assert(t, icmd.Success)
	c.RunDockerCmd("context", "use", "test-context").Assert(t, icmd.Success)

	const projectName = "compose-e2e-demo"

	networkList := c.RunDockerCmd("--context", "default", "network", "ls")

	t.Run("build", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "build", "-f", "../../tests/composefiles/demo_multi_port.yaml")
		res.Assert(t, icmd.Expected{Out: "COPY words.sql /docker-entrypoint-initdb.d/"})
		res.Assert(t, icmd.Expected{Out: "COPY pom.xml ."})
		res.Assert(t, icmd.Expected{Out: "COPY static /static/"})
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "up", "-d", "-f", "../../tests/composefiles/demo_multi_port.yaml", "--project-name", projectName, "-d")
	})

	t.Run("check running project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ps", "-p", projectName)
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:80"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd("--context", "default", "network", "ls")
		assert.Equal(t, len(Lines(res.Stdout())), len(Lines(networkList.Stdout()))+1)
	})

	t.Run("check compose labels", func(t *testing.T) {
		res := c.RunDockerCmd("--context", "default", "inspect", projectName+"_web_1")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "compose-e2e-demo"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "False",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.config-hash":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.config_files": "../../tests/composefiles/demo_multi_port.yaml"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.working_dir":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.service": "web"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version":`})

		res = c.RunDockerCmd("--context", "default", "network", "inspect", projectName+"_default")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.network": "default"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": `})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version": `})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "down", "--project-name", projectName)
	})

	t.Run("check compose labels", func(t *testing.T) {
		networksAfterDown := c.RunDockerCmd("--context", "default", "network", "ls")
		assert.Equal(t, networkList.Stdout(), networksAfterDown.Stdout())
	})
}

func TestLocalComposeVolume(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	c.RunDockerCmd("context", "create", "local", "test-context").Assert(t, icmd.Success)
	c.RunDockerCmd("context", "use", "test-context").Assert(t, icmd.Success)

	const projectName = "compose-e2e-volume"

	t.Run("up with build and no image name, volume", func(t *testing.T) {
		//ensure local test run does not reuse previously build image
		c.RunDockerOrExitError("--context", "default", "rmi", "compose-e2e-volume_nginx")
		c.RunDockerCmd("compose", "up", "-d", "--workdir", "volume-test", "--project-name", projectName)

		output := HTTPGetWithRetry(t, "http://localhost:8090", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, "Hello from Nginx container"))

		_ = c.RunDockerCmd("compose", "down", "--project-name", projectName)
	})
}
