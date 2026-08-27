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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestLocalComposeUp(t *testing.T) {
	// this test shares a fixture with TestCompatibility and can't run at the same time
	c := NewCLI(t)

	const projectName = "compose-e2e-demo"

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/sentences/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check accessing running app", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `web`})

		webPort := c.ServicePublishedPort(t, projectName, "web", 80)
		endpoint := fmt.Sprintf("http://localhost:%d", webPort)
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd(t, "network", "ls")
		res.Assert(t, icmd.Expected{Out: projectName + "_default"})
	})

	t.Run("top", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-p", projectName, "top")
		output := res.Stdout()
		head := []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}
		for _, h := range head {
			assert.Assert(t, strings.Contains(output, h), output)
		}
		assert.Assert(t, strings.Contains(output, `java -Xmx8m -Xms8m -jar /app/words.jar`), output)
		assert.Assert(t, strings.Contains(output, `/dispatcher`), output)
	})

	t.Run("check compose labels", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-web-1")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "compose-e2e-demo"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "False",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.config-hash":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.config_files":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.working_dir":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.service": "web"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version":`})

		res = c.RunDockerCmd(t, "network", "inspect", projectName+"_default")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.network": "default"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": `})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version": `})
	})

	t.Run("check user labels", func(t *testing.T) {
		res := c.RunDockerCmd(t, "inspect", projectName+"-web-1")
		res.Assert(t, icmd.Expected{Out: `"my-label": "test"`})
	})

	t.Run("check healthcheck output", func(t *testing.T) {
		c.WaitForCmdResult(t, c.NewDockerComposeCmd(t, "-p", projectName, "ps", "--format", "json"),
			IsHealthy(projectName+"-web-1"),
			5*time.Second, 1*time.Second)

		res := c.RunDockerComposeCmd(t, "-p", projectName, "ps")
		assertServiceStatus(t, projectName, "web", "(healthy)", res.Stdout())
	})

	t.Run("images", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-p", projectName, "images")
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-db-1      gtardif/sentences-db    latest`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-web-1     gtardif/sentences-web   latest`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-words-1   gtardif/sentences-api   latest`})
	})

	t.Run("down SERVICE", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down", "web")

		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "ps")
		assert.Assert(t, !strings.Contains(res.Combined(), "compose-e2e-demo-web-1"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), "compose-e2e-demo-db-1"), res.Combined())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})

	t.Run("check networks after down", func(t *testing.T) {
		res := c.RunDockerCmd(t, "network", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestDownComposefileInParentFolder(t *testing.T) {
	c := NewParallelCLI(t)

	tmpFolder, err := os.MkdirTemp("fixtures/simple-composefile", "test-tmp")
	assert.NilError(t, err)
	defer os.Remove(tmpFolder) //nolint:errcheck
	projectName := filepath.Base(tmpFolder)

	res := c.RunDockerComposeCmd(t, "--project-directory", tmpFolder, "up", "-d")
	res.Assert(t, icmd.Expected{Err: "Started", ExitCode: 0})

	res = c.RunDockerComposeCmd(t, "-p", projectName, "down")
	res.Assert(t, icmd.Expected{Err: "Removed", ExitCode: 0})
}

func TestAttachRestart(t *testing.T) {
	c := NewParallelCLI(t)

	cmd := c.NewDockerComposeCmd(t, "--ansi=never", "--project-directory", "./fixtures/attach-restart", "up")
	res := icmd.StartCmd(cmd)
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-p", "attach-restart", "down")
	})

	c.WaitForCondition(t, func() (bool, string) {
		debug := res.Combined()
		return strings.Count(res.Stdout(),
				"failing-1 exited with code 1") == 3, fmt.Sprintf("'failing-1 exited with code 1' not found 3 times in : \n%s\n",
				debug)
	}, 4*time.Minute, 2*time.Second)

	// The exit notice comes from the events monitor while the log line comes
	// from the logs stream compose re-attaches after each restart: two
	// asynchronous channels, so the third "world" may land shortly after the
	// third exit notice — wait for it rather than asserting a snapshot.
	c.WaitForCondition(t, func() (bool, string) {
		return strings.Count(res.Stdout(),
				"failing-1  | world") == 3, fmt.Sprintf("'failing-1  | world' not found 3 times in : \n%s\n",
				res.Combined())
	}, time.Minute, time.Second)
}

func TestInitContainer(t *testing.T) {
	NewScenario(t, "a service_completed_successfully dependency must run to completion before its dependent starts").
		Step("up runs the init container first, then the dependent",
			ComposeCmd("--ansi=never", "up", "--menu=false"),
			OutputMatches("foo-1  | hello(?m:.*)bar-1  | world"))
}

func TestRm(t *testing.T) {
	NewScenario(t, "rm --stop --force must remove the selected service's containers, or every service's without selection").
		Step("up starts both services",
			ComposeCmd("up", "-d"),
			ServiceState("simple", "running"),
			ServiceState("another", "running")).
		Step("rm on one service removes only its container",
			ComposeCmd("rm", "--stop", "--force", "simple"),
			ServiceNotCreated("simple"),
			ServiceState("another", "running")).
		Step("up brings the removed service back",
			ComposeCmd("up", "-d"),
			ServiceState("simple", "running")).
		Step("rm without selection removes every container",
			ComposeCmd("rm", "--stop", "--force"),
			ServiceNotCreated("simple"),
			ServiceNotCreated("another"))
}

func TestCompatibility(t *testing.T) {
	s := NewScenario(t, "--compatibility must name containers with underscore separators")
	s.Step("up names the container the v1 way",
		ComposeCmd("--compatibility", "up", "-d"),
		ServiceState("simple", "running")).
		Step("the container name uses underscores",
			DockerCmd("ps", "--filter", "label=com.docker.compose.project="+s.Project(), "--format", "{{.Names}}"),
			OutputContains(s.Project()+"_simple_1"))
}

func TestConfig(t *testing.T) {
	s := NewScenario(t, "config must render the canonical model, resolving the build context to an absolute path")
	s.Step("the rendering resolves the context and names the default network",
		ComposeCmd("config"),
		OutputContains(fmt.Sprintf(`name: %s
services:
  nginx:
    build:
      context: %s
      dockerfile: Dockerfile
    networks:
      default: null
networks:
  default:
    name: %s_default
`, s.Project(), filepath.Join(s.Dir(), "nginx-build"), s.Project())))
}

func TestConfigInterpolate(t *testing.T) {
	s := NewScenario(t, "config --no-interpolate must keep variable expressions while still resolving paths")
	s.Step("the rendering keeps the dockerfile variable",
		ComposeCmd("config", "--no-interpolate"),
		OutputContains(fmt.Sprintf(`name: %s
networks:
  default:
    name: %s_default
services:
  nginx:
    build:
      context: %s
      dockerfile: ${MYVAR}
    networks:
      default: null
`, s.Project(), s.Project(), filepath.Join(s.Dir(), "nginx-build"))))
}

func TestStopWithDependenciesAttached(t *testing.T) {
	NewScenario(t, "up --attach-dependencies must stop with the target service, reporting its exit").
		Step("up returns when the attached service exits",
			ComposeCmd("up", "--attach-dependencies", "foo", "--menu=false").Within(60*time.Second),
			OutputContains("exited with code 0"))
}

func TestRemoveOrphaned(t *testing.T) {
	NewScenario(t, "down --remove-orphans scoped to a service must not touch the other services").
		Step("up starts the stack",
			ComposeCmd("up", "-d"),
			ServiceState("web", "running"),
			ServiceState("words", "running"),
			ServiceState("db", "running")).
		Step("down scoped to one service leaves the others alone",
			ComposeCmd("down", "--remove-orphans", "web"),
			ServiceNotCreated("web"),
			ServiceState("words", "running"),
			ServiceState("db", "running"))
}

func TestComposeFileSetByDotEnv(t *testing.T) {
	c := NewCLI(t)
	defer c.cleanupWithDown(t, "dotenv")

	cmd := c.NewDockerComposeCmd(t, "config")
	cmd.Dir = filepath.Join(".", "fixtures", "dotenv")
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "image: test:latest",
	})
	res.Assert(t, icmd.Expected{
		Out: "image: enabled:profile",
	})
}

func TestComposeFileSetByProjectDirectory(t *testing.T) {
	c := NewCLI(t)
	defer c.cleanupWithDown(t, "dotenv")

	dir := filepath.Join(".", "fixtures", "dotenv", "development")
	cmd := c.NewDockerComposeCmd(t, "--project-directory", dir, "config")
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "image: backend:latest",
	})
}

func TestComposeFileSetByEnvFile(t *testing.T) {
	c := NewCLI(t)
	defer c.cleanupWithDown(t, "dotenv")

	dotEnv, err := os.CreateTemp(t.TempDir(), ".env")
	assert.NilError(t, err)
	err = os.WriteFile(dotEnv.Name(), []byte(`
COMPOSE_FILE=fixtures/dotenv/development/compose.yaml
IMAGE_NAME=test
IMAGE_TAG=latest
COMPOSE_PROFILES=test
`), 0o700)
	assert.NilError(t, err)

	cmd := c.NewDockerComposeCmd(t, "--env-file", dotEnv.Name(), "config")
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		Out: "image: test:latest",
	})
	res.Assert(t, icmd.Expected{
		Out: "image: enabled:profile",
	})
}

func TestNestedDotEnv(t *testing.T) {
	c := NewCLI(t)
	defer c.cleanupWithDown(t, "nested")

	cmd := c.NewDockerComposeCmd(t, "run", "echo")
	cmd.Dir = filepath.Join(".", "fixtures", "nested")
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "root win=root",
	})

	cmd = c.NewDockerComposeCmd(t, "run", "echo")
	cmd.Dir = filepath.Join(".", "fixtures", "nested", "sub")
	defer c.cleanupWithDown(t, "nested")
	res = icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "root sub win=sub",
	})
}

func TestUnnecessaryResources(t *testing.T) {
	NewScenario(t, "a missing external network must only fail the services that use it").
		Step("up with every service is rejected over the missing network",
			ComposeCmd("up", "-d").MayFail(),
			ExitCode(1),
			OutputContains("network foo_bar declared as external, but could not be found")).
		Step("up scoped to the unaffected service succeeds",
			ComposeCmd("up", "-d", "test"),
			ServiceState("test", "running"))
}
