//go:build e2e

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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestLocalComposeConfig(t *testing.T) {
	NewScenario(t, "config must render the resolved model in the requested format, interpolated or not").
		Step("the yaml rendering expands the port shorthand",
			ComposeCmd("config"),
			OutputContains(`
    ports:
      - mode: ingress
        target: 80
        published: "8080"
        protocol: tcp`)).
		Step("the json rendering carries the same resolution",
			ComposeCmd("config", "--format", "json"),
			OutputContains(`"published": "8080"`)).
		Step("--no-interpolate keeps the variable expression",
			ComposeCmd("config", "--no-interpolate"),
			OutputContains(`- ${PORT:-8080}:80`)).
		Step("--no-interpolate warns that service selection is ignored",
			ComposeCmd("config", "--no-interpolate", "test"),
			OutputContains("service filtering is not applied when --no-interpolate is set"),
			OutputContains(`- ${PORT:-8080}:80`)).
		Step("--variables renders the model's variables as json",
			ComposeCmd("config", "--variables", "--format", "json"),
			OutputContains(`{
    "PORT": {
        "Name": "PORT",
        "DefaultValue": "8080",
        "PresenceValue": "",
        "Required": false
    }
}`)).
		Step("--variables renders the model's variables as yaml",
			ComposeCmd("config", "--variables", "--format", "yaml"),
			OutputContains(`PORT:
    name: PORT
    defaultvalue: "8080"
    presencevalue: ""
    required: false`)).
		Step("--variables warns that service selection is ignored",
			ComposeCmd("config", "--variables", "test"),
			OutputContains("service filtering is not applied when --variables is set"),
			OutputContains("PORT"))
}

func TestLocalComposeConfigNoConsistency(t *testing.T) {
	NewScenario(t, "config --no-consistency must list resources of a model that would fail validation").
		Step("--services lists the incomplete service",
			ComposeCmd("config", "--no-consistency", "--services"),
			OutputContains("incomplete")).
		Step("--volumes lists the declared volume",
			ComposeCmd("config", "--no-consistency", "--volumes"),
			OutputContains("data")).
		Step("--networks lists the declared network",
			ComposeCmd("config", "--no-consistency", "--networks"),
			OutputContains("internal")).
		Step("--models lists the declared model",
			ComposeCmd("config", "--no-consistency", "--models"),
			OutputContains("ai/example")).
		Step("--hash still hashes the incomplete service",
			ComposeCmd("config", "--no-consistency", "--hash", "*"),
			OutputContains("incomplete ")).
		Step("--profile exposes the gated service",
			ComposeCmd("--profile", "extra", "config", "--no-consistency", "--services"),
			OutputContains("gated"))
}

func TestConfigServicesFilter(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-config-filter"

	t.Run("--filter profile activates and selects the profile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "profile=workers")
		assert.Equal(t, res.Stdout(), "monitor\nworker\n")
	})

	t.Run("--filter rejects profile wildcard", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "profile=*")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: "profiles must be selected explicitly"})
	})

	t.Run("--filter label", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "label=tier=backend")
		assert.Equal(t, res.Stdout(), "core\n")
	})

	t.Run("--filter combines criteria", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "profile=workers", "--filter", "label=tier=backend")
		assert.Equal(t, res.Stdout(), "worker\n")
	})

	t.Run("--filter no match prints nothing", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "profile=unknown")
		assert.Equal(t, res.Stdout(), "")
	})

	t.Run("--filter requires --services", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--filter", "profile=workers")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: "--filter requires --services"})
	})

	t.Run("--filter rejects unknown criteria", func(t *testing.T) {
		res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/config-filter/compose.yaml", "--project-name", projectName,
			"config", "--services", "--filter", "state=running")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: `unknown criteria "state"`})
	})
}

func TestConfigHashMatchesContainerLabel(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-config-hash"
	defer c.cleanupWithDown(t, projectName)

	c.RunDockerComposeCmd(t, "-f", "./fixtures/config_hash/compose.yaml", "--project-name", projectName, "up", "-d", "--force-recreate")

	containerHashes := map[string]string{}
	for _, service := range []string{"with-env-file", "optional-env", "plain"} {
		res := c.RunDockerCmd(t, "inspect", "-f", `{{index .Config.Labels "com.docker.compose.config-hash"}}`,
			fmt.Sprintf("%s-%s-1", projectName, service))
		containerHashes[service] = strings.TrimSpace(res.Stdout())

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/config_hash/compose.yaml", "--project-name", projectName, "config", "--hash", service)
		fields := strings.Fields(res.Stdout())
		assert.Equal(t, len(fields), 2)
		assert.Equal(t, fields[1], containerHashes[service], "config --hash %s must match the container config-hash label", service)
	}

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config_hash/compose.yaml", "--project-name", projectName, "config", "--hash", "*")
	expected := fmt.Sprintf("optional-env %s\nplain %s\nwith-env-file %s",
		containerHashes["optional-env"], containerHashes["plain"], containerHashes["with-env-file"])
	assert.Equal(t, strings.TrimSpace(res.Stdout()), expected)
}
