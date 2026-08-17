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
	"path/filepath"
	"testing"
)

// The env-priority scenarios all run the same service, whose image ENV says
// "Dockerfile", against different env sources; what the container prints
// tells which source won.

const envPriorityDockerfile = `
-- Dockerfile --
FROM alpine
ENV  WHEREAMI=Dockerfile
CMD ["printenv", "WHEREAMI"]
`

func TestEnvPriorityComposeEnvironment(t *testing.T) {
	s := NewScenario(t, "the caller's environment must beat the compose file's environment section")
	s.Files(`
-- compose.yaml --
services:
  env-compose-priority:
    build:
      context: .
    environment:
      WHEREAMI: "Compose File"
-- .env --
WHEREAMI=Env File
-- .env.override --
WHEREAMI=override
`+envPriorityDockerfile).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-env-compose-priority").MayFail()).
		Step("run -e with the variable set in the shell wins over the environment section",
			ComposeCmd("--env-file", filepath.Join(s.Dir(), ".env.override"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority").WithEnv("WHEREAMI=shell"),
			StdoutContains("shell")).
		Step("run -e with an explicit value wins over the environment section",
			ComposeCmd("--env-file", filepath.Join(s.Dir(), ".env.override"),
				"run", "--rm", "-e", "WHEREAMI=shell", "env-compose-priority"),
			StdoutContains("shell"))
}

func TestEnvPriority(t *testing.T) {
	s := NewScenario(t, "without an environment section, run -e must resolve shell, env-file then image, in that order")
	s.Files(`
-- compose.yaml --
services:
  env-compose-priority:
    build:
      context: .
-- .env --
WHEREAMI=Env File
-- .env.override --
WHEREAMI=override
-- .env.override.with.default --
WHEREAMI=${WHEREAMI:-EnvFileDefaultValue}
-- .env.empty --
` + envPriorityDockerfile)
	dir := s.Dir()
	s.Defer(DockerCmd("image", "rm", "-f", s.Project()+"-env-compose-priority").MayFail()).
		Step("the shell's value wins over an override env file",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.override"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority").WithEnv("WHEREAMI=shell"),
			StdoutContains("shell")).
		Step("the shell's value wins over an env file defaulting the same variable",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.override.with.default"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority").WithEnv("WHEREAMI=shell"),
			StdoutContains("shell")).
		Step("with no shell value, the env file's default value applies",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.override.with.default"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority"),
			StdoutContains("EnvFileDefaultValue")).
		Step("COMPOSE_ENV_FILES designates the env file like --env-file would",
			ComposeCmd("run", "--rm", "-e", "WHEREAMI", "env-compose-priority").
				WithEnv("COMPOSE_ENV_FILES="+filepath.Join(dir, ".env.override.with.default")),
			StdoutContains("EnvFileDefaultValue")).
		Step("an explicit run -e value beats every file",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.override"),
				"run", "--rm", "-e", "WHEREAMI=shell-run", "env-compose-priority"),
			StdoutContains("shell-run")).
		Step("an override env file beats the project's .env",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.override"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority"),
			StdoutContains("override")).
		Step("without flags the project's .env applies",
			ComposeCmd("run", "--rm", "-e", "WHEREAMI", "env-compose-priority"),
			StdoutContains("Env File")).
		Step("with an empty env file the image's ENV survives",
			ComposeCmd("--env-file", filepath.Join(dir, ".env.empty"),
				"run", "--rm", "-e", "WHEREAMI", "env-compose-priority"),
			StdoutContains("Dockerfile"))
}

func TestEnvPriorityComposeEnvFile(t *testing.T) {
	s := NewScenario(t, "the project's .env must feed run -e even when the service declares its own env_file")
	s.Files(`
-- compose.yaml --
services:
  env-compose-priority:
    build:
      context: .
    env_file:
      - .env.override
-- .env --
WHEREAMI=Env File
-- .env.override --
WHEREAMI=override
`+envPriorityDockerfile).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-env-compose-priority").MayFail()).
		Step("run -e forwards the .env value, not the service env_file's",
			ComposeCmd("run", "--rm", "-e", "WHEREAMI", "env-compose-priority"),
			StdoutContains("Env File"))
}

func TestEnvInterpolation(t *testing.T) {
	NewScenario(t, "a shell variable must win over the .env when interpolating the model").
		Files(`
-- compose.yaml --
services:
  env-interpolation:
    image: bash
    environment:
      IMAGE: ${IMAGE}
    command: echo "$IMAGE"
-- .env --
WHEREAMI=EnvFile
IMAGE=default_env:${WHEREAMI}
`).
		Step("config interpolates the image from the shell's value",
			ComposeCmd("config").WithEnv("WHEREAMI=shell"),
			OutputContains("IMAGE: default_env:shell"))
}

func TestEnvInterpolationDefaultValue(t *testing.T) {
	NewScenario(t, "an unset variable must fall back to the .env default when interpolating the model").
		Files(`
-- compose.yaml --
services:
  env-interpolation:
    image: bash
    environment:
      IMAGE: ${IMAGE}
    command: echo "$IMAGE"
-- .env --
IMAGE=default_env:${WHEREAMI:-EnvFileDefaultValue}
`).
		Step("config interpolates the image from the default value",
			ComposeCmd("config"),
			OutputContains("IMAGE: default_env:EnvFileDefaultValue"))
}

func TestCommentsInEnvFile(t *testing.T) {
	s := NewScenario(t, "an unquoted # must start a comment in .env, a quoted # must not")
	s.Files(`
-- compose.yaml --
services:
  env-file-comments:
    build:
      context: .
-- .env --
COMMENT=1234#5
NO_COMMENT="1234#5"
-- Dockerfile --
FROM alpine
ENV  COMMENT=Dockerfile
ENV  NO_COMMENT=Dockerfile
CMD ["sh", "-c", "printenv", "|", "grep", "COMMENT"]
`).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-env-file-comments").MayFail()).
		Step("build the probe image",
			ComposeCmd("build")).
		Step("the comment is stripped unless quoted",
			ComposeCmd("run", "--rm", "-e", "COMMENT", "-e", "NO_COMMENT", "env-file-comments"),
			StdoutContains("COMMENT=1234"),
			StdoutContains("NO_COMMENT=1234#5"))
}

func TestUnsetEnv(t *testing.T) {
	s := NewScenario(t, "an environment passthrough must propagate the caller's value, or unset the image's ENV")
	s.Files(`
-- compose.yaml --
services:
  empty-variable:
    build:
      context: .
    environment:
      - EMPTY # expect to propagate value from user's env OR unset in container
-- Dockerfile --
FROM alpine
ENV  EMPTY=not_empty
CMD ["sh", "-c", "echo \"=$EMPTY=\""]
`).
		Defer(DockerCmd("image", "rm", "-f", s.Project()+"-empty-variable").MayFail()).
		Step("build the probe image",
			ComposeCmd("build")).
		Step("run -e overrides the image's ENV",
			ComposeCmd("run", "-e", "EMPTY=hello", "--rm", "empty-variable"),
			StdoutContains("=hello=")).
		Step("without a caller value the passthrough unsets the image's ENV",
			ComposeCmd("run", "--rm", "empty-variable"),
			StdoutContains("=="))
}
