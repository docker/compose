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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/compose/v2/pkg/utils"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestDisplayScanMessageAfterBuild(t *testing.T) {
	c := NewParallelCLI(t)

	// assert docker scan plugin is available
	c.RunDockerOrExitError(t, "scan", "--help")

	t.Run("display on compose build", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p",
			"scan-msg-test-compose-build", "build")
		defer c.RunDockerOrExitError(t, "rmi", "-f", "scan-msg-test-compose-build-nginx")
		res.Assert(t, icmd.Expected{Err: utils.ScanSuggestMsg})
	})

	t.Run("do not display on compose build with quiet flag", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test-quiet",
			"build", "--quiet")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
		res = c.RunDockerCmd(t, "rmi", "-f", "scan-msg-test-quiet-nginx")
		assert.Assert(t, !strings.Contains(res.Combined(), "No such image"))

		res = c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test-q",
			"build", "-q")
		defer c.RunDockerOrExitError(t, "rmi", "-f", "scan-msg-test-q-nginx")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})

	_ = c.RunDockerOrExitError(t, "rmi", "scan-msg-test-nginx")

	t.Run("display on compose up if image is built", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test", "up",
			"-d")
		defer c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test", "down")
		res.Assert(t, icmd.Expected{Err: utils.ScanSuggestMsg})
	})

	t.Run("do not display on compose up if no image built", func(t *testing.T) { // re-run the same Compose aproject
		res := c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test", "up",
			"-d")
		defer c.RunDockerComposeCmd(t, "-f", "fixtures/simple-build-test/compose.yaml", "-p", "scan-msg-test", "down", "--rmi", "all")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})

	t.Run("do not display if scan already invoked", func(t *testing.T) {
		_ = os.MkdirAll(filepath.Join(c.ConfigDir, "scan"), 0o755)
		scanConfigFile := filepath.Join(c.ConfigDir, "scan", "config.json")
		err := os.WriteFile(scanConfigFile, []byte(`{"optin":true}`), 0o644)
		assert.NilError(t, err)

		res := c.RunDockerCmd(t, "build", "-t", "test-image-scan-msg", "fixtures/simple-build-test/nginx-build")
		assert.Assert(t, !strings.Contains(res.Combined(), "docker scan"), res.Combined())
	})
}
