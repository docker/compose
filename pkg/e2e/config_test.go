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
	"gotest.tools/v3/icmd"
)

func TestLocalComposeConfig(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-config"

	t.Run("yaml", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config")
		res.Assert(t, icmd.Expected{Out: `
    ports:
      - mode: ingress
        target: 80
        published: "8080"
        protocol: tcp`})
	})

	t.Run("json", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--format", "json")
		res.Assert(t, icmd.Expected{Out: `"published": "8080"`})
	})

	t.Run("--no-interpolate", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--no-interpolate")
		res.Assert(t, icmd.Expected{Out: `- ${PORT:-8080}:80`})
	})

	t.Run("--no-interpolate with service selection", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--no-interpolate", "test")
		res.Assert(t, icmd.Expected{
			Err: "service filtering is not applied when --no-interpolate is set",
			Out: `- ${PORT:-8080}:80`,
		})
	})

	t.Run("--variables --format json", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--variables", "--format", "json")
		res.Assert(t, icmd.Expected{Out: `{
    "PORT": {
        "Name": "PORT",
        "DefaultValue": "8080",
        "PresenceValue": "",
        "Required": false
    }
}`})
	})

	t.Run("--variables --format yaml", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--variables", "--format", "yaml")
		res.Assert(t, icmd.Expected{Out: `PORT:
    name: PORT
    defaultvalue: "8080"
    presencevalue: ""
    required: false`})
	})

	t.Run("--variables with service selection", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/compose.yaml", "--project-name", projectName, "config", "--variables", "test")
		res.Assert(t, icmd.Expected{
			Err: "service filtering is not applied when --variables is set",
			Out: `PORT`,
		})
	})

	t.Run("--no-consistency --services", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "config", "--no-consistency", "--services")
		res.Assert(t, icmd.Expected{Out: `incomplete`})
	})

	t.Run("--no-consistency --volumes", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "config", "--no-consistency", "--volumes")
		res.Assert(t, icmd.Expected{Out: `data`})
	})

	t.Run("--no-consistency --networks", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "config", "--no-consistency", "--networks")
		res.Assert(t, icmd.Expected{Out: `internal`})
	})

	t.Run("--no-consistency --models", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "config", "--no-consistency", "--models")
		res.Assert(t, icmd.Expected{Out: `ai/example`})
	})

	t.Run("--no-consistency --hash", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "config", "--no-consistency", "--hash", "*")
		res.Assert(t, icmd.Expected{Out: `incomplete `})
	})

	t.Run("--profile --no-consistency --services", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/config/no-consistency.yaml", "--project-name", projectName, "--profile", "extra", "config", "--no-consistency", "--services")
		res.Assert(t, icmd.Expected{Out: `gated`})
	})
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
