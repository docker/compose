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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

const variablesProject = "compose-e2e-variables"

func TestComposeVariablesSimple(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/simple/compose.yaml",
		"--project-name", variablesProject+"-simple",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:1.4.2"})
	res.Assert(t, icmd.Expected{Out: "image: redis:7.4"})
}

func TestComposeVariablesFile(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/expanded/compose.yaml",
		"--project-name", variablesProject+"-expanded",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:1.4.2"})
	res.Assert(t, icmd.Expected{Out: "image: redis:7.0"})
}

func TestComposeVariablesCrossRef(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/cross-ref/compose.yaml",
		"--project-name", variablesProject+"-crossref",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: acme/api:1.4.2"})
}

func TestComposeVariablesIncludeLocal(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/include-local/compose.yaml",
		"--project-name", variablesProject+"-include",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: redis:7.4"})
	res.Assert(t, icmd.Expected{Out: "image: postgres:16.2"})
}

func TestComposeVariablesNoLeakage(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/no-leakage/compose.yaml",
		"--project-name", variablesProject+"-noleak",
		"config",
	)
	// redis sees include-local MODULE
	res.Assert(t, icmd.Expected{Out: "image: redis:redis-only"})
	// postgres falls through to default
	res.Assert(t, icmd.Expected{Out: "image: postgres:empty"})
}

func TestComposeVariablesCLIOverride(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/cli-override/compose.yaml",
		"--project-name", variablesProject+"-cli",
		"--var", "APP_VERSION=2.0.0",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:2.0.0"})
}

func TestComposeVariablesCLIVarFile(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/cli-var-file/compose.yaml",
		"--project-name", variablesProject+"-clivarfile",
		"--var-file", "./fixtures/variables/cli-var-file/overrides.yaml",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:from-cli-var-file"})
}

func TestComposeVariablesCLIVarBeatsCLIVarFile(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/cli-var-file/compose.yaml",
		"--project-name", variablesProject+"-clibeatsfile",
		"--var-file", "./fixtures/variables/cli-var-file/overrides.yaml",
		"--var", "APP_VERSION=from-flag",
		"config",
	)
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:from-flag"})
}

func TestComposeVariablesShellEnvWins(t *testing.T) {
	c := NewParallelCLI(t)
	res := icmd.RunCmd(c.NewDockerComposeCmd(t,
		"-f", "./fixtures/variables/cli-override/compose.yaml",
		"--project-name", variablesProject+"-shell",
		"--var", "APP_VERSION=from-cli",
		"config",
	), func(c *icmd.Cmd) { c.Env = append(c.Env, "APP_VERSION=from-shell") })
	res.Assert(t, icmd.Expected{Out: "image: ghcr.io/acme/api:from-shell"})
}

func TestComposeVariablesCoercion(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/coercion/compose.yaml",
		"--project-name", variablesProject+"-coerce",
		"config",
	)
	out := res.Stdout()
	// Compose normalizes the short port form into a long form.
	assert.Assert(t, strings.Contains(out, "published: \"8080\""))
	assert.Assert(t, strings.Contains(out, "target: 80"))
	assert.Assert(t, strings.Contains(out, "ENABLED: \"true\""))
	assert.Assert(t, strings.Contains(out, "image: api:1.4.2"))
}

func TestComposeVariablesConfigVariablesShowsResolved(t *testing.T) {
	c := NewParallelCLI(t)
	res := c.RunDockerComposeCmd(t,
		"-f", "./fixtures/variables/simple/compose.yaml",
		"--project-name", variablesProject+"-confvars",
		"config", "--variables",
	)
	out := res.Stdout()
	assert.Assert(t, strings.Contains(out, "RESOLVED VALUE"))
	assert.Assert(t, strings.Contains(out, "SOURCE"))
	assert.Assert(t, strings.Contains(out, "1.4.2"))
	assert.Assert(t, strings.Contains(out, "root-inline"))
}
