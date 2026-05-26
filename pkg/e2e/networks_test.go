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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"
)

func TestNetworks(t *testing.T) {
	// fixture is shared with TestNetworkModes and is not safe to run concurrently
	const projectName = "network-e2e"
	c := NewCLI(t, WithEnv(
		"COMPOSE_PROJECT_NAME="+projectName,
		"COMPOSE_FILE=./fixtures/network-test/compose.yaml",
	))

	c.RunDockerComposeCmd(t, "down", "-t0", "-v")

	c.RunDockerComposeCmd(t, "up", "-d")

	res := c.RunDockerComposeCmd(t, "ps")
	res.Assert(t, icmd.Expected{Out: `web`})

	endpoint := "http://localhost:80"
	output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
	assert.Assert(t, strings.Contains(output, `"word":`))

	res = c.RunDockerCmd(t, "network", "ls")
	res.Assert(t, icmd.Expected{Out: projectName + "_dbnet"})
	res.Assert(t, icmd.Expected{Out: "microservices"})

	res = c.RunDockerComposeCmd(t, "port", "words", "8080")
	res.Assert(t, icmd.Expected{Out: `0.0.0.0:8080`})

	c.RunDockerComposeCmd(t, "down", "-t0", "-v")
	res = c.RunDockerCmd(t, "network", "ls")
	assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	assert.Assert(t, !strings.Contains(res.Combined(), "microservices"), res.Combined())
}

func TestNetworkAliases(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "network_alias_e2e"
	defer c.cleanupWithDown(t, projectName)

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName, "up",
			"-d")
	})

	t.Run("curl alias", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName,
			"exec", "-T", "container1", "curl", "http://alias-of-container2/")
		assert.Assert(t, strings.Contains(res.Stdout(), "Welcome to nginx!"), res.Stdout())
	})

	t.Run("curl links", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-alias/compose.yaml", "--project-name", projectName,
			"exec", "-T", "container1", "curl", "http://container/")
		assert.Assert(t, strings.Contains(res.Stdout(), "Welcome to nginx!"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestNetworkLinks(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "network_link_e2e"

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/network-links/compose.yaml", "--project-name", projectName, "up",
			"-d")
	})

	t.Run("curl links in default bridge network", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-links/compose.yaml", "--project-name", projectName,
			"exec", "-T", "container2", "curl", "http://container1/")
		assert.Assert(t, is.Contains(res.Stdout(), "Welcome to nginx!"))
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestIPAMConfig(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "ipam_e2e"

	t.Run("ensure we do not reuse previous networks", func(t *testing.T) {
		c.RunDockerOrExitError(t, "network", "rm", projectName+"_default")
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/ipam/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("ensure service get fixed IP assigned", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-foo-1", "-f",
			fmt.Sprintf(`{{ $network := index .NetworkSettings.Networks "%s_default" }}{{ $network.IPAMConfig.IPv4Address }}`, projectName))
		res.Assert(t, icmd.Expected{Out: "10.1.0.100"})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})
}

func TestNetworkModes(t *testing.T) {
	// fixture is shared with TestNetworks and is not safe to run concurrently
	c := NewCLI(t)

	const projectName = "network_mode_service_run"
	defer c.cleanupWithDown(t, projectName)

	t.Run("run with service mode dependency", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-test/compose.yaml", "--project-name", projectName, "run", "-T", "mydb", "echo", "success")
		res.Assert(t, icmd.Expected{Out: "success"})
	})
}

func TestNetworkConfigChanged(t *testing.T) {
	// fixture is shared with TestNetworks and is not safe to run concurrently
	c := NewCLI(t)
	const projectName = "network_config_change"

	c.RunDockerComposeCmd(t, "-f", "./fixtures/network-test/compose.subnet.yaml", "--project-name", projectName, "up", "-d")
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	res := c.RunDockerComposeCmd(t, "--project-name", projectName, "exec", "test", "hostname", "-i")
	res.Assert(t, icmd.Expected{Out: "172.99.0."})
	res.Combined()

	cmd := c.NewCmdWithEnv([]string{"SUBNET=192.168.0.0/16"},
		"docker", "compose", "-f", "./fixtures/network-test/compose.subnet.yaml", "--project-name", projectName, "up", "-d")
	res = icmd.RunCmd(cmd)
	res.Assert(t, icmd.Success)
	out := res.Combined()
	fmt.Println(out)

	res = c.RunDockerComposeCmd(t, "--project-name", projectName, "exec", "test", "hostname", "-i")
	res.Assert(t, icmd.Expected{Out: "192.168.0."})
}

func TestMacAddress(t *testing.T) {
	c := NewCLI(t)
	const projectName = "network_mac_address"
	c.RunDockerComposeCmd(t, "-f", "./fixtures/network-test/mac_address.yaml", "--project-name", projectName, "up", "-d")
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})
	res := c.RunDockerCmd(t, "inspect", fmt.Sprintf("%s-test-1", projectName), "-f", "{{ (index .NetworkSettings.Networks \"network_mac_address_default\" ).MacAddress }}")
	res.Assert(t, icmd.Expected{Out: "00:e0:84:35:d0:e8"})
}

func TestInterfaceName(t *testing.T) {
	c := NewCLI(t)

	version := c.RunDockerCmd(t, "version", "-f", "{{.Server.Version}}")
	major, _, found := strings.Cut(version.Combined(), ".")
	assert.Assert(t, found)
	if major == "26" || major == "27" {
		t.Skip("Skipping test due to docker version < 28")
	}

	const projectName = "network_interface_name"
	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-interface-name/compose.yaml", "--project-name", projectName, "run", "test")
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})
	res.Assert(t, icmd.Expected{Out: "foobar@"})
}

func TestNetworkRecreate(t *testing.T) {
	c := NewCLI(t)
	const projectName = "network_recreate"
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
	})
	c.RunDockerComposeCmd(t, "-f", "./fixtures/network-recreate/compose.yaml", "--project-name", projectName, "up", "-d")

	c = NewCLI(t, WithEnv("FOO=bar"))
	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-recreate/compose.yaml", "--project-name", projectName, "--progress=plain", "up", "-d")
	err := res.Stderr()
	fmt.Println(err)
	hasStopped := strings.Contains(err, "Stopped")
	hasResumed := strings.Contains(err, "Started") || strings.Contains(err, "Recreated")
	if !hasStopped || !hasResumed {
		t.Fatalf("unexpected output, missing expected events, stderr: %s", err)
	}
}

func TestExternalNetworkAliases(t *testing.T) {
	const projectName = "network_external_alias_e2e"
	const externalNet = projectName + "_external"

	c := NewParallelCLI(t, WithEnv("EXTERNAL_NETWORK="+externalNet))

	c.RunDockerOrExitError(t, "network", "rm", externalNet)
	c.RunDockerCmd(t, "network", "create", externalNet)
	t.Cleanup(func() {
		c.cleanupWithDown(t, projectName)
		c.RunDockerOrExitError(t, "network", "rm", externalNet)
	})

	upRes := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-external-alias/compose.yaml",
		"--project-name", projectName,
		"up", "-d")

	internalNet := projectName + "_default"

	t.Run("warning lists services without explicit external-net alias and excludes self-aliased ones", func(t *testing.T) {
		var warningLine string
		for line := range strings.SplitSeq(upRes.Combined(), "\n") {
			if strings.Contains(line, `not registered as aliases on external network`) {
				warningLine = line
				break
			}
		}
		assert.Assert(t, warningLine != "", "expected warning line in output:\n%s", upRes.Combined())
		assert.Assert(t, strings.Contains(warningLine, "web"), warningLine)
		assert.Assert(t, strings.Contains(warningLine, "external-net"), warningLine)
		assert.Assert(t, !strings.Contains(warningLine, "db"),
			"db declares its own external-net alias and must be excluded from the warning: %s", warningLine)
	})

	t.Run("service name is an alias on internal network", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-web-1", "-f",
			fmt.Sprintf(`{{ range (index .NetworkSettings.Networks %q).Aliases }}[{{ . }}]{{ end }}`, internalNet))
		assert.Assert(t, strings.Contains(res.Stdout(), "[web]"), res.Stdout())
	})

	t.Run("service name is not an alias on external network", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-web-1", "-f",
			fmt.Sprintf(`{{ range (index .NetworkSettings.Networks %q).Aliases }}[{{ . }}]{{ end }}`, externalNet))
		assert.Assert(t, !strings.Contains(res.Stdout(), "[web]"), res.Stdout())
	})

	t.Run("explicit alias under networks.<external>.aliases is registered on external network", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-db-1", "-f",
			fmt.Sprintf(`{{ range (index .NetworkSettings.Networks %q).Aliases }}[{{ . }}]{{ end }}`, externalNet))
		assert.Assert(t, strings.Contains(res.Stdout(), "[db]"), res.Stdout())
	})

	t.Run("service name resolves on internal network", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/network-external-alias/compose.yaml",
			"--project-name", projectName,
			"exec", "-T", "web", "ping", "-c1", "db")
		assert.Assert(t, strings.Contains(res.Combined(), "1 packets transmitted"), res.Combined())
	})
}
