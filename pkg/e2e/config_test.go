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
	"testing"

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
}
