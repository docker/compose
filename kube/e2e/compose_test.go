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
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	. "github.com/docker/compose-cli/utils/e2e"
)

var binDir string

func TestMain(m *testing.M) {
	p, cleanup, err := SetupExistingCLI()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	binDir = p
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestComposeUp(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "compose-kube-demo"

	t.Run("create kube context", func(t *testing.T) {
		res := c.RunDockerCmd("context", "create", "kubernetes", "--kubeconfig", "/Users/gtardif/.kube/config", "--kubecontext", "docker-desktop", "kube-e2e")
		res.Assert(t, icmd.Expected{Out: `Successfully created kube context "kube-e2e"`})
		c.RunDockerCmd("context", "use", "kube-e2e")
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./kube-simple-demo/demo_sentences.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("check running project", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `web`})

		endpoint := "http://localhost:95"
		output := HTTPGetWithRetry(t, endpoint+"/words/noun", http.StatusOK, 2*time.Second, 20*time.Second)
		assert.Assert(t, strings.Contains(output, `"word":`))
	})
	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})

	t.Run("check containers after down", func(t *testing.T) {
		res := c.RunDockerCmd("ps", "--all")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}
