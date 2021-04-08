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
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/utils/e2e"
)

func TestDisplayScanMessageAfterBuild(t *testing.T) {

	c := NewParallelE2eCLI(t, binDir)
	setupScanPlugin(t, c)

	res := c.RunDockerCmd("info")
	res.Assert(t, icmd.Expected{Out: "scan: Docker Scan"})

	t.Run("display when docker build", func(t *testing.T) {
		res := c.RunDockerCmd("build", "-t", "test-image-scan-msg", "./fixtures/simple-build-test/nginx-build")
		res.Assert(t, icmd.Expected{Out: "Use 'docker scan' to run Snyk tests against images to find vulnerabilities and learn how to fix them"})
	})

	t.Run("do not display if envvar  DOCKER_SCAN_SUGGEST=false", func(t *testing.T) {
		cmd := c.NewDockerCmd("build", "-t", "test-image-scan-msg", "./fixtures/build-test/nginx-build")
		cmd.Env = append(cmd.Env, "DOCKER_SCAN_SUGGEST=false")
		res := icmd.StartCmd(cmd)
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})

	t.Run("display on compose build", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yml", "-p", "scan-msg-test", "build")
		res.Assert(t, icmd.Expected{Out: "Use 'docker scan' to run Snyk tests against images to find vulnerabilities and learn how to fix them"})
	})

	_ = c.RunDockerOrExitError("rmi", "scan-msg-test_nginx")

	t.Run("display on compose up if image is built", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yml", "-p", "scan-msg-test", "up", "-d")
		defer c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yml", "-p", "scan-msg-test", "down")
		res.Assert(t, icmd.Expected{Out: "Use 'docker scan' to run Snyk tests against images to find vulnerabilities and learn how to fix them"})
	})

	t.Run("do not display on compose up if no image built", func(t *testing.T) { // re-run the same Compose aproject
		res := c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yml", "-p", "scan-msg-test", "up", "-d")
		defer c.RunDockerCmd("compose", "-f", "./fixtures/simple-build-test/compose.yml", "-p", "scan-msg-test", "down")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})

	t.Run("do not display if scan already invoked", func(t *testing.T) {
		_ = os.MkdirAll(filepath.Join(c.ConfigDir, "scan"), 0755)
		scanConfigFile := filepath.Join(c.ConfigDir, "scan", "config.json")
		err := ioutil.WriteFile(scanConfigFile, []byte(`{"optin":true}`), 0644)
		assert.NilError(t, err)

		res := c.RunDockerCmd("build", "-t", "test-image-scan-msg", "./fixtures/simple-build-test/nginx-build")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})
}

func setupScanPlugin(t *testing.T, c *E2eCLI) {
	_ = os.MkdirAll(filepath.Join(c.ConfigDir, "cli-plugins"), 0755)

	scanPluginFile := "docker-scan"
	scanPluginURL := "https://github.com/docker/scan-cli-plugin/releases/download/v0.7.0/docker-scan_linux_amd64"
	if runtime.GOOS == "windows" {
		scanPluginFile += ".exe"
		scanPluginURL = "https://github.com/docker/scan-cli-plugin/releases/download/v0.7.0/docker-scan_windows_amd64.exe"
	}
	if runtime.GOOS == "darwin" {
		scanPluginURL = "https://github.com/docker/scan-cli-plugin/releases/download/v0.7.0/docker-scan_darwin_amd64"
	}

	localScanBinary := filepath.Join("..", "..", "..", "bin", scanPluginFile)
	if _, err := os.Stat(localScanBinary); os.IsNotExist(err) {
		out, err := os.Create(localScanBinary)
		assert.NilError(t, err)
		defer out.Close() //nolint:errcheck
		resp, err := http.Get(scanPluginURL)
		assert.NilError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		_, err = io.Copy(out, resp.Body)
		assert.NilError(t, err)
	}

	finalScanBinaryFile := filepath.Join(c.ConfigDir, "cli-plugins", scanPluginFile)

	out, err := os.Create(finalScanBinaryFile)
	assert.NilError(t, err)
	defer out.Close() //nolint:errcheck
	in, err := os.Open(localScanBinary)
	assert.NilError(t, err)
	defer in.Close() //nolint:errcheck
	_, err = io.Copy(out, in)
	assert.NilError(t, err)

	err = os.Chmod(finalScanBinaryFile, 7777)
	assert.NilError(t, err)
}
