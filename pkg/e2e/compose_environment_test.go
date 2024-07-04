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

func TestEnvPriority(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("up", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "env-compose-priority")
		c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose-with-env.yaml",
			"up", "-d", "--build")
	})

	// Full options activated
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From OS Environment)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("compose file priority", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose-with-env.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell")
	})

	// Full options activated
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("compose file priority", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose-with-env.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override",
			"run", "--rm", "-e", "WHEREAMI=shell", "env-compose-priority")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell")
	})

	// No Compose file, all other options
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From OS Environment)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("shell priority", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell")
	})

	// No Compose file, all other options with env variable from OS environment
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("shell priority file with default value", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override.with.default",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell")
	})

	// No Compose file, all other options with env variable from OS environment
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment default value from file in --env-file)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("shell priority implicitly set", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override.with.default",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "EnvFileDefaultValue")
	})

	// No Compose file, all other options with env variable from OS environment
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment default value from file in COMPOSE_ENV_FILES)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("shell priority from COMPOSE_ENV_FILES variable", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "COMPOSE_ENV_FILES=./fixtures/environment/env-priority/.env.override.with.default")
		res := icmd.RunCmd(cmd)
		stdout := res.Stdout()
		assert.Equal(t, strings.TrimSpace(stdout), "EnvFileDefaultValue")
	})

	// No Compose file and env variable pass to the run command
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("shell priority from run command", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override",
			"run", "--rm", "-e", "WHEREAMI=shell-run", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell-run")
	})

	// No Compose file & no env variable but override env file
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment patched by .env as a default --env-file value)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("override env file from compose", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose-with-env-file.yaml",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Env File")
	})

	// No Compose file & no env variable but override by default env file
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment patched by --env-file value)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("override env file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.override",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "override")
	})

	// No Compose file & no env variable but override env file
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)  <-- Result expected (From environment patched by --env-file value)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive
	// 5. Variable is not defined
	t.Run("env file", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Env File")
	})

	// No Compose file & no env variable, using an empty override env file
	// 1. Command Line (docker compose run --env <KEY[=VAL]>)
	// 2. Compose File (service::environment section)
	// 3. Compose File (service::env_file section file)
	// 4. Container Image ENV directive <-- Result expected
	// 5. Variable is not defined
	t.Run("use Dockerfile", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-priority/compose.yaml",
			"--env-file", "./fixtures/environment/env-priority/.env.empty",
			"run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Dockerfile")
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--project-name", "env-priority", "down")
	})
}

func TestEnvInterpolation(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("shell priority from run command", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd(t, "-f", "./fixtures/environment/env-interpolation/compose.yaml", "config")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{Out: `IMAGE: default_env:shell`})
	})

	t.Run("shell priority from run command using default value fallback", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-interpolation-default-value/compose.yaml", "config").
			Assert(t, icmd.Expected{Out: `IMAGE: default_env:EnvFileDefaultValue`})
	})
}

func TestCommentsInEnvFile(t *testing.T) {
	c := NewParallelCLI(t)

	t.Run("comments in env files", func(t *testing.T) {
		c.RunDockerOrExitError(t, "rmi", "env-file-comments")

		c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-file-comments/compose.yaml", "up", "-d", "--build")

		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/env-file-comments/compose.yaml",
			"run", "--rm", "-e", "COMMENT", "-e", "NO_COMMENT", "env-file-comments")

		res.Assert(t, icmd.Expected{Out: `COMMENT=1234`})
		res.Assert(t, icmd.Expected{Out: `NO_COMMENT=1234#5`})

		c.RunDockerComposeCmd(t, "--project-name", "env-file-comments", "down", "--rmi", "all")
	})
}

func TestUnsetEnv(t *testing.T) {
	c := NewParallelCLI(t)
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "--project-name", "empty-variable", "down", "--rmi", "all")
	})

	t.Run("override env variable", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/empty-variable/compose.yaml", "build")

		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/empty-variable/compose.yaml",
			"run", "-e", "EMPTY=hello", "--rm", "empty-variable")
		res.Assert(t, icmd.Expected{Out: `=hello=`})
	})

	t.Run("unset env variable", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/environment/empty-variable/compose.yaml",
			"run", "--rm", "empty-variable")
		res.Assert(t, icmd.Expected{Out: `==`})
	})
}
