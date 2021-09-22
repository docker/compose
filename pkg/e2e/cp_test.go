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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

func TestCopy(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)

	const projectName = "copy_e2e"

	t.Cleanup(func() {
		c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "--project-name", projectName, "down")

		os.Remove("./fixtures/cp-test/from-default.txt") //nolint:errcheck
		os.Remove("./fixtures/cp-test/from-indexed.txt") //nolint:errcheck
		os.RemoveAll("./fixtures/cp-test/cp-folder2")    //nolint:errcheck
	})

	t.Run("start service", func(t *testing.T) {
		c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "--project-name", projectName, "up", "--scale", "nginx=5", "-d")
	})

	t.Run("make sure service is running", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-p", projectName, "ps")
		res.Assert(t, icmd.Expected{Out: `nginx               running`})
	})

	t.Run("copy to container copies the file to the first container by default", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "./fixtures/cp-test/cp-me.txt", "nginx:/tmp/default.txt")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		output := c.RunDockerCmd("exec", projectName+"-nginx-1", "cat", "/tmp/default.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world`), output)

		res = c.RunDockerOrExitError("exec", projectName+"_nginx_2", "cat", "/tmp/default.txt")
		res.Assert(t, icmd.Expected{ExitCode: 1})
	})

	t.Run("copy to container with a given index copies the file to the given container", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "--index=3", "./fixtures/cp-test/cp-me.txt", "nginx:/tmp/indexed.txt")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		output := c.RunDockerCmd("exec", projectName+"-nginx-3", "cat", "/tmp/indexed.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world`), output)

		res = c.RunDockerOrExitError("exec", projectName+"-nginx-2", "cat", "/tmp/indexed.txt")
		res.Assert(t, icmd.Expected{ExitCode: 1})
	})

	t.Run("copy to container with the all flag copies the file to all containers", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "--all", "./fixtures/cp-test/cp-me.txt", "nginx:/tmp/all.txt")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		output := c.RunDockerCmd("exec", projectName+"-nginx-1", "cat", "/tmp/all.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world`), output)

		output = c.RunDockerCmd("exec", projectName+"-nginx-2", "cat", "/tmp/all.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world`), output)

		output = c.RunDockerCmd("exec", projectName+"-nginx-3", "cat", "/tmp/all.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world`), output)
	})

	t.Run("copy from a container copies the file to the host from the first container by default", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "nginx:/tmp/default.txt", "./fixtures/cp-test/from-default.txt")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		data, err := os.ReadFile("./fixtures/cp-test/from-default.txt")
		assert.NilError(t, err)
		assert.Equal(t, `hello world`, string(data))
	})

	t.Run("copy from a container with a given index copies the file to host", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "--index=3", "nginx:/tmp/indexed.txt", "./fixtures/cp-test/from-indexed.txt")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		data, err := os.ReadFile("./fixtures/cp-test/from-indexed.txt")
		assert.NilError(t, err)
		assert.Equal(t, `hello world`, string(data))
	})

	t.Run("copy to and from a container also work with folder", func(t *testing.T) {
		res := c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "./fixtures/cp-test/cp-folder", "nginx:/tmp")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		output := c.RunDockerCmd("exec", projectName+"-nginx-1", "cat", "/tmp/cp-folder/cp-me.txt").Stdout()
		assert.Assert(t, strings.Contains(output, `hello world from folder`), output)

		res = c.RunDockerCmd("compose", "-f", "./fixtures/cp-test/compose.yaml", "-p", projectName, "cp", "nginx:/tmp/cp-folder", "./fixtures/cp-test/cp-folder2")
		res.Assert(t, icmd.Expected{ExitCode: 0})

		data, err := os.ReadFile("./fixtures/cp-test/cp-folder2/cp-me.txt")
		assert.NilError(t, err)
		assert.Equal(t, `hello world from folder`, string(data))
	})
}
