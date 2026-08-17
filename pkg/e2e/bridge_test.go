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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

const bridgeImageVersion = "v0.0.3"

func TestConvertAndTransformList(t *testing.T) {
	c := NewParallelCLI(t)

	const projectName = "bridge"
	tmpDir := t.TempDir()

	t.Run("kubernetes manifests", func(t *testing.T) {
		kubedir := filepath.Join(tmpDir, "kubernetes")
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/bridge/compose.yaml", "--project-name", projectName, "bridge", "convert",
			"--output", kubedir, "--transformation", fmt.Sprintf("docker/compose-bridge-kubernetes:%s", bridgeImageVersion))
		assert.NilError(t, res.Error)
		assert.Equal(t, res.ExitCode, 0)
		res = c.RunCmd(t, "diff", "-r", kubedir, "./fixtures/bridge/expected-kubernetes")
		assert.NilError(t, res.Error, res.Combined())
	})

	t.Run("helm charts", func(t *testing.T) {
		helmDir := filepath.Join(tmpDir, "helm")
		res := c.RunDockerComposeCmd(t, "-f", "./fixtures/bridge/compose.yaml", "--project-name", projectName, "bridge", "convert",
			"--output", helmDir, "--transformation", fmt.Sprintf("docker/compose-bridge-helm:%s", bridgeImageVersion))
		assert.NilError(t, res.Error)
		assert.Equal(t, res.ExitCode, 0)
		res = c.RunCmd(t, "diff", "-r", helmDir, "./fixtures/bridge/expected-helm")
		assert.NilError(t, res.Error, res.Combined())
	})

	t.Run("list transformers images", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "bridge", "transformations",
			"ls")
		assert.Assert(t, strings.Contains(res.Stdout(), "docker/compose-bridge-helm"), res.Combined())
		assert.Assert(t, strings.Contains(res.Stdout(), "docker/compose-bridge-kubernetes"), res.Combined())
	})
}

func TestConvertBuildOnlyService(t *testing.T) {
	c := NewParallelCLI(t)
	outDir := t.TempDir()

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/bridge-build-only/compose.yaml", "--project-name", "bridge-build-only", "bridge", "convert",
		"--output", outDir, "--transformation", fmt.Sprintf("docker/compose-bridge-kubernetes:%s", bridgeImageVersion))
	assert.NilError(t, res.Error)
	assert.Equal(t, res.ExitCode, 0)

	entries, err := os.ReadDir(outDir)
	assert.NilError(t, err)
	assert.Assert(t, len(entries) > 0, "expected bridge conversion to produce output")
}
