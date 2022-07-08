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

	projectDir := "./fixtures/environment/env-priority"

	t.Run("up", func(t *testing.T) {
		c.RunDockerOrExitError("rmi", "env-compose-priority")
		c.RunDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose-with-env.yaml",
			"--project-directory", projectDir, "up", "-d", "--build")
	})

	// Full options activated
	// 1. Compose file <-- Result expected
	// 2. Shell environment variables
	// 3. Environment file
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("compose file priority", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose-with-env.yaml",
			"--project-directory", projectDir, "--env-file", "./fixtures/environment/env-priority/.env.override", "run",
			"--rm", "-e", "WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Compose File")
	})

	// No Compose file, all other options
	// 1. Compose file
	// 2. Shell environment variables <-- Result expected
	// 3. Environment file
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("shell priority", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose.yaml", "--project-directory",
			projectDir, "--env-file", "./fixtures/environment/env-priority/.env.override", "run", "--rm", "-e",
			"WHEREAMI", "env-compose-priority")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell")
	})

	//  No Compose file and env variable pass to the run command
	// 1. Compose file
	// 2. Shell environment variables <-- Result expected
	// 3. Environment file
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("shell priority from run command", func(t *testing.T) {
		res := c.RunDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose.yaml", "--project-directory",
			projectDir, "--env-file", "./fixtures/environment/env-priority/.env.override", "run", "--rm", "-e",
			"WHEREAMI=shell-run", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "shell-run")
	})

	//  No Compose file & no env variable but override env file
	// 1. Compose file
	// 2. Shell environment variables
	// 3. Environment file <-- Result expected
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("override env file", func(t *testing.T) {
		res := c.RunDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose.yaml", "--project-directory",
			projectDir, "--env-file", "./fixtures/environment/env-priority/.env.override", "run", "--rm", "-e",
			"WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "override")
	})

	//  No Compose file & no env variable but override env file
	// 1. Compose file
	// 2. Shell environment variables
	// 3. Environment file <-- Result expected
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("env file", func(t *testing.T) {
		res := c.RunDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose.yaml", "--project-directory",
			projectDir, "run", "--rm", "-e", "WHEREAMI", "env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Env File")
	})

	//  No Compose file & no env variable, using an empty override env file
	// 1. Compose file
	// 2. Shell environment variables
	// 3. Environment file
	// 4. Dockerfile   <-- Result expected
	// 5. Variable is not defined
	t.Run("use Dockerfile", func(t *testing.T) {
		res := c.RunDockerComposeCmd("-f", "./fixtures/environment/env-priority/compose.yaml", "--project-directory",
			projectDir, "--env-file", "./fixtures/environment/env-priority/.env.empty", "run", "--rm", "-e", "WHEREAMI",
			"env-compose-priority")
		assert.Equal(t, strings.TrimSpace(res.Stdout()), "Dockerfile")
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd("--project-directory", projectDir, "down")
	})
}

func TestEnvInterpolation(t *testing.T) {
	c := NewParallelCLI(t)

	projectDir := "./fixtures/environment/env-interpolation"

	//  No variable defined in the Compose file and env variable pass to the run command
	// 1. Compose file
	// 2. Shell environment variables <-- Result expected
	// 3. Environment file
	// 4. Dockerfile
	// 5. Variable is not defined
	t.Run("shell priority from run command", func(t *testing.T) {
		cmd := c.NewDockerComposeCmd("-f", "./fixtures/environment/env-interpolation/compose.yaml",
			"--project-directory", projectDir, "config")
		cmd.Env = append(cmd.Env, "WHEREAMI=shell")
		res := icmd.RunCmd(cmd)
		res.Assert(t, icmd.Expected{Out: `IMAGE: default_env:shell`})
	})
}

func TestCommentsInEnvFile(t *testing.T) {
	c := NewParallelCLI(t)

	projectDir := "./fixtures/environment/env-file-comments"

	t.Run("comments in env files", func(t *testing.T) {
		c.RunDockerOrExitError("rmi", "env-file-comments")

		c.RunDockerComposeCmd("-f", "./fixtures/environment/env-file-comments/compose.yaml", "--project-directory",
			projectDir, "up", "-d", "--build")

		res := c.RunDockerComposeCmd("-f", "./fixtures/environment/env-file-comments/compose.yaml",
			"--project-directory", projectDir, "run", "--rm", "-e", "COMMENT", "-e", "NO_COMMENT", "env-file-comments")

		res.Assert(t, icmd.Expected{Out: `COMMENT=1234`})
		res.Assert(t, icmd.Expected{Out: `NO_COMMENT=1234#5`})

		c.RunDockerComposeCmd("--project-directory", projectDir, "down", "--rmi", "all")
	})
}
