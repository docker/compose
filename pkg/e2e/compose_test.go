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
	"io/ioutil"
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

var binDir string

func TestMain(m *testing.M) {
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestLocalComposeUp(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-demo"

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/sentences/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check accessing running app", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:90"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))

		res = c.RunDockerCmd("network", "ls")
		res.Assert(t, icmd.Expected{Out: projectName + "_default"})
	})

	t.Run("top", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "top")
		output := res.Stdout()
		head := []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}
		for _, h := range head {
			assert.Assert(t, strings.Contains(output, h), output)
		}
		assert.Assert(t, strings.Contains(output, `java -Xmx8m -Xms8m -jar /app/words.jar`), output)
		assert.Assert(t, strings.Contains(output, `/dispatcher`), output)
	})

	t.Run("check compose labels", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"-web-1")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.container-number": "1"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": "compose-e2e-demo"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.oneoff": "False",`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.config-hash":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.config_files":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project.working_dir":`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.service": "web"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version":`})

		res = c.RunDockerCmd("network", "inspect", projectName+"_default")
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.network": "default"`})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.project": `})
		res.Assert(t, icmd.Expected{Out: `"com.docker.compose.version": `})
	})

	t.Run("check user labels", func(t *testing.T) {
		res := c.RunDockerCmd("inspect", projectName+"-web-1")
		res.Assert(t, icmd.Expected{Out: `"my-label": "test"`})

	})

	t.Run("check healthcheck output", func(t *testing.T) {
		c.WaitForCmdResult(c.NewDockerCmd("compose", "-p", projectName, "ps", "--format", "json"),
			StdoutContains(`"Name":"compose-e2e-demo-web-1","Command":"/dispatcher","Project":"compose-e2e-demo","Service":"web","State":"running","Health":"healthy"`),
			5*time.Second, 1*time.Second)

		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `NAME                       COMMAND                  SERVICE             STATUS              PORTS`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-web-1     "/dispatcher"            web                 running (healthy)   0.0.0.0:90->80/tcp, :::90->80/tcp`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-db-1      "docker-entrypoint.s…"   db                  running             5432/tcp`})
	})

	t.Run("images", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "images")
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-db-1      gtardif/sentences-db    latest`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-web-1     gtardif/sentences-web   latest`})
		res.Assert(t, icmd.Expected{Out: `compose-e2e-demo-words-1   gtardif/sentences-api   latest`})
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})

	t.Run("check networks after down", func(t *testing.T) {
		res := c.RunDockerCmd("network", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}

func TestComposePull(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	res := c.RunDockerOrExitError("compose", "--project-directory", "fixtures/simple-composefile", "pull")
	output := res.Combined()

	assert.Assert(t, strings.Contains(output, "simple Pulled"))
	assert.Assert(t, strings.Contains(output, "another Pulled"))
}

func TestDownComposefileInParentFolder(t *testing.T) {

	c := NewParallelE2eCLI(t, binDir)

	tmpFolder, err := ioutil.TempDir("fixtures/simple-composefile", "test-tmp")
	assert.NilError(t, err)
	defer os.Remove(tmpFolder) // nolint: errcheck
	projectName := filepath.Base(tmpFolder)

	res := c.RunDockerCmd("compose", "--project-directory", tmpFolder, "up", "-d")
	res.Assert(t, icmd.Expected{Err: "Started", ExitCode: 0})

	res = c.RunDockerCmd("compose", "-p", projectName, "down")
	res.Assert(t, icmd.Expected{Err: "Removed", ExitCode: 0})
}

func TestAttachRestart(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	cmd := c.NewDockerCmd("compose", "--ansi=never", "--project-directory", "./fixtures/attach-restart", "up")
	res := icmd.StartCmd(cmd)
	defer c.RunDockerOrExitError("compose", "-p", "attach-restart", "down")

	c.WaitForCondition(func() (bool, string) {
		debug := res.Combined()
		return strings.Count(res.Stdout(), "failing-1 exited with code 1") == 3, fmt.Sprintf("'failing-1 exited with code 1' not found 3 times in : \n%s\n", debug)
	}, 2*time.Minute, 2*time.Second)

	assert.Equal(t, strings.Count(res.Stdout(), "failing-1  | world"), 3, res.Combined())
}

func TestInitContainer(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	res := c.RunDockerOrExitError("compose", "--ansi=never", "--project-directory", "./fixtures/init-container", "up")
	defer c.RunDockerOrExitError("compose", "-p", "init-container", "down")
	testify.Regexp(t, "foo-1  | hello(?m:.*)bar-1  | world", res.Stdout())
}

func TestRm(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-rm"

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "up", "-d")
	})

	t.Run("rm -sf", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/simple-composefile/compose.yaml", "-p", projectName, "rm", "-sf", "simple")
		res.Assert(t, icmd.Expected{Err: "Removed", ExitCode: 0})
	})

	t.Run("check containers after rm -sf", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName+"_simple"), res.Combined())
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerCmd("compose", "-p", projectName, "down")
	})
}

func TestCompatibility(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-e2e-compatibility"

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "--compatibility", "-f", "./fixtures/sentences/compose.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check container names", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--format", "{{.Names}}")
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_web_1"})
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_words_1"})
		res.Assert(t, icmd.Expected{Out: "compose-e2e-compatibility_db_1"})
	})

	t.Run("down", func(t *testing.T) {
		c.RunDockerCmd("compose", "-p", projectName, "down")
	})
}

func TestConvert(t *testing.T) {
	const projectName = "compose-e2e-convert"
	c := NewParallelE2eCLI(t, binDir)

	wd, err := os.Getwd()
	assert.NilError(t, err)

	t.Run("up", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yaml", "-p", projectName, "convert")
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf(`services:
  nginx:
    build:
      context: %s
      dockerfile: Dockerfile
    networks:
      default: null
networks:
  default:
    name: compose-e2e-convert_default`, filepath.Join(wd, "fixtures", "simple-build-test", "nginx-build")), ExitCode: 0})
	})
}
