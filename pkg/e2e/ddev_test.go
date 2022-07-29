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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

const ddevVersion = "v1.19.1"

func TestComposeRunDdev(t *testing.T) {
	if !composeStandaloneMode {
		t.Skip("Not running in plugin mode - ddev only supports invoking standalone `docker-compose`")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Running on Windows. Skipping...")
	}

	// ddev shells out to `docker` and `docker-compose` (standalone), so a
	// temporary directory is created with symlinks to system Docker and the
	// locally-built standalone Compose binary to use as PATH
	requiredTools := []string{
		findToolInPath(t, DockerExecutableName),
		ComposeStandalonePath(t),
		findToolInPath(t, "tar"),
		findToolInPath(t, "gzip"),
	}
	pathDir := t.TempDir()
	for _, tool := range requiredTools {
		require.NoError(t, os.Symlink(tool, filepath.Join(pathDir, filepath.Base(tool))),
			"Could not create symlink for %q", tool)
	}

	c := NewCLI(t, WithEnv(
		"DDEV_DEBUG=true",
		fmt.Sprintf("PATH=%s", pathDir),
	))

	ddevDir := t.TempDir()
	siteName := filepath.Base(ddevDir)

	t.Cleanup(func() {
		_ = c.RunCmdInDir(t, ddevDir, "./ddev", "delete", "-Oy")
		_ = c.RunCmdInDir(t, ddevDir, "./ddev", "poweroff")
	})

	osName := "linux"
	if runtime.GOOS == "darwin" {
		osName = "macos"
	}

	compressedFilename := fmt.Sprintf("ddev_%s-%s.%s.tar.gz", osName, runtime.GOARCH, ddevVersion)
	c.RunCmdInDir(t, ddevDir, "curl", "-LO", fmt.Sprintf("https://github.com/drud/ddev/releases/download/%s/%s",
		ddevVersion,
		compressedFilename))

	c.RunCmdInDir(t, ddevDir, "tar", "-xzf", compressedFilename)

	// Create a simple index.php we can test against.
	c.RunCmdInDir(t, ddevDir, "sh", "-c", "echo '<?php\nprint \"ddev is working\";' >index.php")

	c.RunCmdInDir(t, ddevDir, "./ddev", "config", "--auto")
	c.RunCmdInDir(t, ddevDir, "./ddev", "config", "global", "--use-docker-compose-from-path")
	vRes := c.RunCmdInDir(t, ddevDir, "./ddev", "version")
	out := vRes.Stdout()
	fmt.Printf("ddev version: %s\n", out)

	c.RunCmdInDir(t, ddevDir, "./ddev", "poweroff")

	c.RunCmdInDir(t, ddevDir, "./ddev", "start", "-y")

	curlRes := c.RunCmdInDir(t, ddevDir, "curl", "-sSL", fmt.Sprintf("http://%s.ddev.site", siteName))
	out = curlRes.Stdout()
	fmt.Println(out)
	assert.Assert(t, strings.Contains(out, "ddev is working"), "Could not start project")
}

func findToolInPath(t testing.TB, name string) string {
	t.Helper()
	binPath, err := exec.LookPath(name)
	require.NoError(t, err, "Could not find %q in path", name)
	return binPath
}
