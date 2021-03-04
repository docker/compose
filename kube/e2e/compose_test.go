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
	"time"

	testify "github.com/stretchr/testify/assert"
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
	kubeconfig := filepath.Join(c.ConfigDir, "kubeconfig")
	kindClusterName := "e2e"
	kubeContextName := "kind-" + kindClusterName
	dockerContextName := "kube-e2e-ctx"

	t.Run("create kube cluster", func(t *testing.T) {
		c.RunCmd("kind", "create", "cluster", "--name", kindClusterName, "--kubeconfig", kubeconfig, "--wait", "180s")
	})
	defer func() {
		c.RunDockerCmd("context", "use", "default")
		c.RunCmd("kind", "delete", "cluster", "--name", kindClusterName, "--kubeconfig", kubeconfig)
	}()

	t.Run("create kube context", func(t *testing.T) {
		res := c.RunDockerCmd("context", "create", "kubernetes", "--kubeconfig", kubeconfig, "--kubecontext", kubeContextName, dockerContextName)
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf("Successfully created kube context %q", dockerContextName)})
		c.RunDockerCmd("context", "use", dockerContextName)
		res = c.RunDockerCmd("context", "ls")
		res.Assert(t, icmd.Expected{Out: fmt.Sprintf("%s *      kube                %s (in %s)", dockerContextName, kubeContextName, kubeconfig)})
	})

	t.Run("up", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./kube-simple-demo/demo_sentences.yaml", "--project-name", projectName, "up", "-d")
	})

	t.Run("compose ls", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ls", "--format", "json")
		res.Assert(t, icmd.Expected{Out: `[{"Name":"compose-kube-demo","Status":"deployed"}]`})
	})

	t.Run("compose ps --all", func(t *testing.T) {
		getServiceRegx := func(service string) string {
			// match output with random hash / spaces like:
			// db-698f4dd798-jd9gw      db                  Running
			return fmt.Sprintf("%s-.*\\s+%s\\s+Running\\s+", service, service)
		}
		res := c.RunDockerCmd("compose", "-p", projectName, "ps", "--all")
		testify.Regexp(t, getServiceRegx("db"), res.Stdout())
		testify.Regexp(t, getServiceRegx("words"), res.Stdout())
		testify.Regexp(t, getServiceRegx("web"), res.Stdout())

		assert.Equal(t, len(Lines(res.Stdout())), 4, res.Stdout())
	})

	// to be revisited
	/*t.Run("compose ps hides non running containers", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		assert.Equal(t, len(Lines(res.Stdout())), 1, res.Stdout())
	})*/

	t.Run("check running project", func(t *testing.T) {
		// Docker Desktop kube cluster automatically exposes ports on the host, this is not the case with kind on Desktop,
		//we need to connect to the clusterIP, from the kind container
		res := c.RunCmd("sh", "-c", "kubectl --kubeconfig "+kubeconfig+" get service/web -o json | jq -r '.spec.clusterIP'")
		clusterIP := strings.ReplaceAll(strings.TrimSpace(res.Stdout()), `"`, "")

		endpoint := fmt.Sprintf("http://%s:80/words/noun", clusterIP)
		c.WaitForCmdResult(icmd.Command("docker", "--context", "default", "exec", "e2e-control-plane", "curl", endpoint), StdoutContains(`"word":`), 3*time.Minute, 3*time.Second)
	})

	t.Run("compose logs web", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "--project-name", projectName, "logs", "web")
		assert.Assert(t, strings.Contains(res.Stdout(), "Listening on port 80"), res.Stdout())
	})

	t.Run("down", func(t *testing.T) {
		_ = c.RunDockerCmd("compose", "--project-name", projectName, "down")
	})

	t.Run("check stack after down", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "ls")
		assert.Assert(t, !strings.Contains(res.Combined(), projectName), res.Combined())
	})
}
