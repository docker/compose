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
	"gotest.tools/v3/icmd"
)

func TestNetworks(t *testing.T) {
	// fixture is not safe to run concurrently: it binds fixed host ports
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
	NewScenario(t, "a network alias and a link must both resolve to the target service").
		Step("up starts both services",
			ComposeCmd("up", "-d"),
			ServiceState("container1", "running"),
			ServiceState("container2", "running")).
		Step("the network alias resolves",
			ComposeCmd("exec", "-T", "container1", "curl", "http://alias-of-container2/"),
			OutputContains("Welcome to nginx!")).
		Step("the link name resolves",
			ComposeCmd("exec", "-T", "container1", "curl", "http://container/"),
			OutputContains("Welcome to nginx!"))
}

func TestNetworkLinks(t *testing.T) {
	NewScenario(t, "links must resolve between services running on the default bridge network").
		Step("up starts both services",
			ComposeCmd("up", "-d"),
			ServiceState("container1", "running"),
			ServiceState("container2", "running")).
		Step("the linked name resolves over the bridge network",
			ComposeCmd("exec", "-T", "container2", "curl", "http://container1/"),
			OutputContains("Welcome to nginx!"))
}

func TestIPAMConfig(t *testing.T) {
	s := NewScenario(t, "a fixed ipv4_address must be assigned to the service's container")
	s.Step("up starts the service",
		ComposeCmd("up", "-d"),
		ServiceState("foo", "running")).
		Step("the container got the fixed IP",
			DockerCmd("inspect", s.Project()+"-foo-1", "-f",
				fmt.Sprintf(`{{ $network := index .NetworkSettings.Networks "%s_default" }}{{ $network.IPAMConfig.IPv4Address }}`, s.Project())),
			OutputContains("10.1.0.100"))
}

func TestNetworkModes(t *testing.T) {
	NewScenario(t, "run must start a service whose network_mode points at another service").
		Step("run starts the network provider and shares its namespace",
			ComposeCmd("run", "-T", "mydb", "echo", "success"),
			OutputContains("success"),
			ServiceState("db", "running"))
}

func TestNetworkConfigChanged(t *testing.T) {
	NewScenario(t, "a network subnet change must recreate the network and re-address the attached containers").
		Step("up addresses the service in the default subnet",
			ComposeCmd("up", "-d"),
			ServiceState("test", "running")).
		Step("the container's IP belongs to the default subnet",
			ComposeCmd("exec", "test", "hostname", "-i"),
			OutputContains("172.99.0.")).
		Step("up with a changed subnet replaces the network, reconnecting the same container",
			ComposeCmd("up", "-d").WithEnv("SUBNET=192.168.0.0/16"),
			ServiceState("test", "running"),
			NotRecreated("test")).
		Step("the container's IP moved to the new subnet",
			ComposeCmd("exec", "test", "hostname", "-i"),
			OutputContains("192.168.0."))
}

func TestMacAddress(t *testing.T) {
	s := NewScenario(t, "a declared mac_address must be applied to the container's network endpoint")
	s.Step("up starts the service",
		ComposeCmd("up", "-d"),
		ServiceState("test", "running")).
		Step("the endpoint carries the declared mac address",
			DockerCmd("inspect", s.Project()+"-test-1", "-f",
				fmt.Sprintf(`{{ (index .NetworkSettings.Networks "%s_default" ).MacAddress }}`, s.Project())),
			OutputContains("00:e0:84:35:d0:e8"))
}

func TestInterfaceName(t *testing.T) {
	NewScenario(t, "a declared interface_name must name the network interface inside the container").
		Requires(EngineVersionAtLeast(28)).
		Step("the container sees its interface under the declared name",
			ComposeCmd("run", "test"),
			OutputContains("foobar@"))
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
