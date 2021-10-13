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

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestNetworks(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "network_e2e"

	t.Run("ensure we do not reuse previous networks", func(t *testing.T) {
		c.RunDockerOrExitError("network", "rm", projectName+"_dbnet")
		c.RunDockerOrExitError("network", "rm", "microservices")
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/network-test/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check running project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:80"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd("network", "ls")
		res.Assert(t, icmd.Expected{Out: projectName + "_dbnet"})
		res.Assert(t, icmd.Expected{Out: "microservices"})
	})

	t.Run("port", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "--project-name", projectName, "port", "words", "8080")
		res.Assert(t, icmd.Expected{Out: `0.0.0.0:8080`})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})

	t.Run("check networks after down", func(t *testing.T) {
		res := c.RunDockerCmd("network", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), "microservices"), res.Combined())
	})
}

func TestNetworkAliassesAndLinks(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "network_alias_e2e"

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("curl alias", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName, "exec", "-T", "container1", "curl", "http://alias-of-container2/")
		assert.Assert(t, strings.Contains(res.Stdout(), "Welcome to nginx!"), res.Stdout())
	})

	t.Run("curl links", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName, "exec", "-T", "container1", "curl", "http://container/")
		assert.Assert(t, strings.Contains(res.Stdout(), "Welcome to nginx!"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
}

func TestIPAMConfig(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "ipam_e2e"

	t.Run("ensure we do not reuse previous networks", func(t *testing.T) {
		c.RunDockerOrExitError("network", "rm", projectName+"_default")
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/ipam/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("ensure service get fixed IP assigned", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"-foo-1", "-f", "{{ .NetworkSettings.Networks."+projectName+"_default.IPAddress }}")
		res.Assert(t, icmd.Expected{Out: "10.1.0.100"})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
}

func TestNetworkModes(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "network_mode_service_run"

	t.Run("run with service mode dependency", func(t *testing.T) {
		res := c.RunDockerOrExitError("compose", "-f", "./fixtures/network-test/compose.yaml", "--project-name", projectName, "run", "mydb", "echo", "success")
		res.Assert(t, icmd.Expected{Out: "success"})

	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})
}
