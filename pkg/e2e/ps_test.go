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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/compose/v2/pkg/api"
)

func TestPs(t *testing.T) {
	c := NewParallelCLI(t)
	const projectName = "e2e-ps"

	res := c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "up", "-d")
	if assert.NoError(t, res.Error) {
		t.Cleanup(func() {
			_ = c.RunDockerComposeCmd(t, "--project-name", projectName, "down")
		})
	}

	assert.Contains(t, res.Combined(), "Container e2e-ps-busybox-1  Started", res.Combined())

	t.Run("pretty", func(t *testing.T) {
		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps")
		lines := strings.Split(res.Stdout(), "\n")
		assert.Equal(t, 4, len(lines))
		count := 0
		for _, line := range lines[1:3] {
			if strings.Contains(line, "e2e-ps-busybox-1") {
				assert.True(t, strings.Contains(line, "127.0.0.1:8001->8000/tcp"))
				count++
			}
			if strings.Contains(line, "e2e-ps-nginx-1") {
				assert.True(t, strings.Contains(line, "80/tcp, 443/tcp, 8080/tcp"))
				count++
			}
		}
		assert.Equal(t, 2, count, "Did not match both services:\n"+res.Combined())
	})

	t.Run("json", func(t *testing.T) {
		res = c.RunDockerComposeCmd(t, "-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps",
			"--format", "json")
		var output []api.ContainerSummary
		err := json.Unmarshal([]byte(res.Stdout()), &output)
		require.NoError(t, err, "Failed to unmarshal ps JSON output")

		count := 0
		assert.Equal(t, 2, len(output))
		for _, service := range output {
			publishers := service.Publishers
			if service.Name == "e2e-ps-busybox-1" {
				assert.Equal(t, 1, len(publishers))
				assert.Equal(t, api.PortPublishers{
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
				assert.Equal(t, 3, len(publishers))
				assert.Equal(t, api.PortPublishers{
					{TargetPort: 80, Protocol: "tcp"},
					{TargetPort: 443, Protocol: "tcp"},
					{TargetPort: 8080, Protocol: "tcp"},
				}, publishers)

				count++
			}
		}
		assert.Equal(t, 2, count, "Did not match both services:\n"+res.Combined())
	})
}
