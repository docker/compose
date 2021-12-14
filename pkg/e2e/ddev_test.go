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
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

const ddevVersion = "v1.18.2"

func TestComposeRunDdev(t *testing.T) {
	if !composeStandaloneMode {
		t.Skip("Not running on standalone mode.")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Running on Windows. Skipping...")
	}
	c := NewParallelE2eCLI(t, binDir)
	dir, err := os.MkdirTemp("", t.Name()+"-")
	assert.NilError(t, err)

	siteName := filepath.Base(dir)

	t.Cleanup(func() {
		_ = c.RunCmdInDir(dir, "./ddev", "delete", "-Oy")
		_ = c.RunCmdInDir(dir, "./ddev", "poweroff")
		_ = os.RemoveAll(dir)
	})

	osName := "linux"
	if runtime.GOOS == "darwin" {
		osName = "macos"
	}

	compressedFilename := fmt.Sprintf("ddev_%s-%s.%s.tar.gz", osName, runtime.GOARCH, ddevVersion)
	c.RunCmdInDir(dir, "curl", "-LO",
		fmt.Sprintf("https://github.com/drud/ddev/releases/download/%s/%s",
			ddevVersion,
			compressedFilename))

	c.RunCmdInDir(dir, "tar", "-xzf", compressedFilename)
	c.RunDockerCmd("pull", "drud/ddev-ssh-agent:v1.18.0")
	c.RunDockerCmd("pull", "busybox:stable")
	c.RunDockerCmd("pull", "phpmyadmin:5")

	c.RunDockerCmd("pull", tagged("drud/ddev-router"))
	c.RunDockerCmd("pull", tagged("drud/ddev-dbserver-mariadb-10.3"))
	c.RunDockerCmd("pull", tagged("drud/ddev-webserver"))

	// Create a simple index.php we can test against.
	c.RunCmdInDir(dir, "sh", "-c", "echo '<?php\nprint \"ddev is working\";' >index.php")

	c.RunCmdInDir(dir, "./ddev", "config", "--auto")
	c.RunCmdInDir(dir, "./ddev", "config", "global", "--use-docker-compose-from-path")

	c.RunCmdInDir(dir, "./ddev", "poweroff")

	startRes := c.RunCmdInDir(dir, "./ddev", "start", "-y")
	assert.Equal(c.test, startRes.ExitCode, 0, "Could not start project")

	curlRes := c.RunCmdInDir(dir, "curl", "-sSL", fmt.Sprintf("http://%s.ddev.site", siteName))
	out := curlRes.Stdout()
	fmt.Println(out)
	assert.Assert(c.test, strings.Contains(out, "ddev is working"), "Could not start project")
}

func tagged(img string) string {
	return fmt.Sprintf("%s:%s", img, ddevVersion)
}
