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

	testify "github.com/stretchr/testify/assert"
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

		endpoint := "http://localhost:90"
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
	t.Skip("Skipping test until we can fix it")

	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("Skipping test on CI... flaky")
	}
	c := NewParallelCLI(t)

	cmd := c.NewDockerComposeCmd(t, "--ansi=never", "--project-directory", "./fixtures/attach-restart", "up")
	res := icmd.StartCmd(cmd)
	defer c.RunDockerComposeCmd(t, "-p", "attach-restart", "down")

	c.WaitForCondition(t, func() (bool, string) {
		debug := res.Combined()
		return strings.Count(res.Stdout(),
				"failing-1 exited with code 1") == 3, fmt.Sprintf("'failing-1 exited with code 1' not found 3 times in : \n%s\n",
				debug)
	}, 4*time.Minute, 2*time.Second)

	assert.Equal(t, strings.Count(res.Stdout(), "failing-1  | world"), 3, res.Combined())
}

func TestInitContainer(t *testing.T) {
	c := NewParallelCLI(t)

	res := c.RunDockerComposeCmd(t, "--ansi=never", "--project-directory", "./fixtures/init-container", "up", "--menu=false")
	defer c.RunDockerComposeCmd(t, "-p", "init-container", "down")
	testify.Regexp(t, "foo-1  | hello(?m:.*)bar-1  | world", res.Stdout())
}

func TestRm(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "compose-e2e-rm"

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "up", "-d")
	})

	t.Run("rm --stop --force simple", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "rm",
			"--stop", "--force", "simple")
		res.Assert(t, icmd.Expected{Err: "Removed", ExitCode: 0})
	})

	t.Run("check containers after rm", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName+"-simple"), res.Combined())
		assert.Assert(t, strings.Contains(res.Combined(), projectName+"-another"), res.Combined())
	})

	t.Run("up (again)", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "up", "-d")
	})

	t.Run("rm ---stop --force <none>", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "rm",
			"--stop", "--force")
		res.Assert(t, icmd.Expected{ExitCode: 0})
	})

	t.Run("check containers after rm", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName+"-simple"), res.Combined())
		assert.Assert(t, !strings.Contains(res.Combined(), projectName+"-another"), res.Combined())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-p", projectName, "down")
	})
}

func TestCompatibility(t *testing.T) {
	// this test shares a fixture with TestLocalComposeUp and can't run at the same time
	c := NewCLI(t)

	const projectName = "compose-e2e-compatibility"

	t.Run("up", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "--compatibility", "-f", "./fixtures/sentences/compose.yaml", "--project-name",
			projectName, "up", "-d")
	})

	t.Run("check container names", func(t *testing.T) {
		res := c.RunDockerCmd(t, "ps", "--format", "{{.Names}}")
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_web_1"})
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_words_1"})
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_db_1"})
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerComposeCmd(t, "-p", projectName, "down")
	})
}

func TestConfig(t *testing.T) {
	const projectName = "compose-e2e-convert"
	c := NewParallelCLI(t)

	wd, err := os.Getwd()
	assert.NilError(t, err)

	t.Run("up", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-build-test/compose.yaml", "-p", projectName, "convert")
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf(`name: %s
services:
  nginx:
    build:
      context: %s
      dockerfile: Dockerfile
    networks:
      default: null
networks:
  default:
    name: compose-e2e-convert_default
`, projectName, filepath.Join(wd, "fixtures", "simple-build-test", "nginx-build")), ExitCode: 0})
	})
}

func TestConfigInterpolate(t *testing.T) {
	const projectName = "compose-e2e-convert-interpolate"
	c := NewParallelCLI(t)

	wd, err := os.Getwd()
	assert.NilError(t, err)

	t.Run("convert", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/simple-build-test/compose-interpolate.yaml", "-p", projectName, "convert", "--no-interpolate")
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf(`name: %s
networks:
  default:
    name: compose-e2e-convert-interpolate_default
services:
  nginx:
    build:
      context: %s
      dockerfile: ${MYVAR}
    networks:
      default: null
`, projectName, filepath.Join(wd, "fixtures", "simple-build-test", "nginx-build")), ExitCode: 0})
	})
}

func TestStopWithDependenciesAttached(t *testing.T) {
	const projectName = "compose-e2e-stop-with-deps"
	c := NewParallelCLI(t, WithEnv("COMMAND=echo hello"))

	cleanup := func() {
		c.RunDockerComposeCmd(t, "-p", projectName, "down", "--remove-orphans", "--timeout=0")
	}
	cleanup()
	t.Cleanup(cleanup)

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/dependencies/compose.yaml", "-p", projectName, "up", "--attach-dependencies", "foo", "--menu=false")
	res.Assert(t, icmd.Expected{Out: "exited with code 0"})
}

func TestRemoveOrphaned(t *testing.T) {
	const projectName = "compose-e2e-remove-orphaned"
	c := NewParallelCLI(t)

	cleanup := func() {
		c.RunDockerComposeCmd(t, "-p", projectName, "down", "--remove-orphans", "--timeout=0")
	}
	cleanup()
	t.Cleanup(cleanup)

	// run stack
	c.RunDockerComposeCmd(t, "-f", "./fixtures/sentences/compose.yaml", "-p", projectName, "up", "-d")

	// down "web" service with orphaned removed
	c.RunDockerComposeCmd(t, "-f", "./fixtures/sentences/compose.yaml", "-p", projectName, "down", "--remove-orphans", "web")

	// check "words" service has not been considered orphaned
	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/sentences/compose.yaml", "-p", projectName, "ps", "--format", "{{.Name}}")
	res.Assert(t, icmd.Expected{Out: fmt.Sprintf("%s-words-1", projectName)})
}

func TestComposeFileSetByDotEnv(t *testing.T) {
	c := NewCLI(t)

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

	cmd := c.NewDockerComposeCmd(t, "run", "echo")
	cmd.Dir = filepath.Join(".", "fixtures", "nested")
	res := icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "root win=root",
	})

	cmd = c.NewDockerComposeCmd(t, "run", "echo")
	cmd.Dir = filepath.Join(".", "fixtures", "nested", "sub")
	res = icmd.RunCmd(cmd)
	res.Assert(t, icmd.Expected{
		ExitCode: 0,
		Out:      "root sub win=sub",
	})

}

func TestUnnecesaryResources(t *testing.T) {
	const projectName = "compose-e2e-unnecessary-resources"
	c := NewParallelCLI(t)
	t.Cleanup(func() {
		c.RunDockerComposeCmd(t, "-p", projectName, "down", "-t=0")
	})

	res := c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/external/compose.yaml", "-p", projectName, "up", "-d")
	res.Assert(t, icmd.Expected{
		ExitCode: 1,
		Err:      "network foo_bar declared as external, but could not be found",
	})

	c.RunDockerComposeCmd(t, "-f", "./fixtures/external/compose.yaml", "-p", projectName, "up", "-d", "test")
	// Should not fail as missing external network is not used
}
