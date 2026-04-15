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
	"encoding/json"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/icmd"

	"github.com/docker/compose/v5/pkg/api"
)

func TestPs(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-ps"

	// ensure clean state from any previous failed run
	c.RunDockerComposeCmdNoCheck(t, "--project-name", projectName, "down", "--remove-orphans")

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "up", "-d")
	assert.NilError(t, res.Error)
	t.Cleanup(func() {
		_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
	})

	assert.Assert(t, is.Contains(res.Combined(), "Container e2e-ps-busybox-1 Started"))

	t.Run("table", func(t *testing.T) {
		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps")
		lines := strings.Split(res.Stdout(), "\n")
		assert.Assert(t, is.Len(lines, 4))
		count := 0
		for _, line := range lines[1:3] {
			if strings.Contains(line, "e2e-ps-busybox-1") {
				assert.Assert(t, is.Contains(line, "127.0.0.1:8001->8000/tcp"))
				count++
			}
			if strings.Contains(line, "e2e-ps-nginx-1") {
				assert.Assert(t, is.Contains(line, "80/tcp, 443/tcp, 8080/tcp"))
				count++
			}
		}
		assert.Equal(t, 2, count, "Did not match both services:\n"+res.Combined())
	})

	t.Run("json", func(t *testing.T) {
		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps",
			"--format", "json")
		type element struct {
			Name       string
			Project    string
			Publishers api.PortPublishers
		}
		var output []element
		out := res.Stdout()
		dec := json.NewDecoder(strings.NewReader(out))
		for dec.More() {
			var s element
			assert.NilError(t, dec.Decode(&s), "Failed to unmarshal ps JSON output")
			output = append(output, s)
		}

		count := 0
		assert.Assert(t, is.Len(output, 2))
		for _, service := range output {
			assert.Equal(t, projectName, service.Project)
			publishers := service.Publishers
			if service.Name == "e2e-ps-busybox-1" {
				assert.Assert(t, is.Len(publishers, 1))
				assert.DeepEqual(t, api.PortPublishers{
					{
						URL:           "127.0.0.1",
						TargetPort:    8000,
						PublishedPort: 8001,
						Protocol:      "tcp",
					},
				}, publishers)
				count++
			}
			if service.Name == "e2e-ps-nginx-1" {
				assert.Assert(t, is.Len(publishers, 3))
				assert.DeepEqual(t, api.PortPublishers{
					{TargetPort: 80, Protocol: "tcp"},
					{TargetPort: 443, Protocol: "tcp"},
					{TargetPort: 8080, Protocol: "tcp"},
				}, publishers)

				count++
			}
		}
		assert.Equal(t, 2, count, "Did not match both services:\n"+res.Combined())
	})

	t.Run("ps --all", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop")
		assert.NilError(t, res.Error)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps")
		lines := strings.Split(res.Stdout(), "\n")
		assert.Assert(t, is.Len(lines, 2))

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps", "--all")
		lines = strings.Split(res.Stdout(), "\n")
		assert.Assert(t, is.Len(lines, 4))
	})

	t.Run("ps unknown", func(t *testing.T) {
		res := c.RunDockerComposeCmd(t, "--project-name", projectName, "stop")
		assert.NilError(t, res.Error)

		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps", "nginx")
		res.Assert(t, icmd.Success)

		res = c.RunDockerComposeCmdNoCheck(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps", "unknown")
		res.Assert(t, icmd.Expected{ExitCode: 1, Err: "no such service: unknown"})
	})
}
