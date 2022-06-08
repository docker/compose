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
)

func TestPs(t *testing.T) {
	c := NewParallelE2eCLI(t, binDir)
	const projectName = "e2e-ps"

	res := c.RunDockerComposeCmd("-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "up", "-d")
	if assert.NoError(t, res.Error) {
		t.Cleanup(func() {
			_ = c.RunDockerComposeCmd("--project-name", projectName, "down")
		})
	}

	assert.Contains(t, res.Combined(), "Container e2e-ps-busybox-1  Started", res.Combined())

	t.Run("pretty", func(t *testing.T) {
		res = c.RunDockerComposeCmd("-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps")
		lines := strings.Split(res.Combined(), "\n")
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
		res = c.RunDockerComposeCmd("-f", "./fixtures/ps-test/compose.yaml", "--project-name", projectName, "ps", "--format", "json")
		var output []map[string]interface{}
		err := json.Unmarshal([]byte(res.Combined()), &output)
		assert.NoError(t, err)

		count := 0
		assert.Equal(t, 2, len(output))
		for _, service := range output {
			publishers := service["Publishers"].([]interface{})
			if service["Name"] == "e2e-ps-busybox-1" {
				assert.Equal(t, 1, len(publishers))
				publisher := publishers[0].(map[string]interface{})
				assert.Equal(t, "127.0.0.1", publisher["URL"])
				assert.Equal(t, 8000.0, publisher["TargetPort"])
				assert.Equal(t, 8001.0, publisher["PublishedPort"])
				assert.Equal(t, "tcp", publisher["Protocol"])
				count++
			}
			if service["Name"] == "e2e-ps-nginx-1" {
				assert.Equal(t, 3, len(publishers))
				publisher := publishers[0].(map[string]interface{})
				assert.Equal(t, 80.0, publisher["TargetPort"])
				assert.Equal(t, 0.0, publisher["PublishedPort"])
				assert.Equal(t, "tcp", publisher["Protocol"])
				publisher = publishers[1].(map[string]interface{})
				assert.Equal(t, 443.0, publisher["TargetPort"])
				assert.Equal(t, 0.0, publisher["PublishedPort"])
				assert.Equal(t, "tcp", publisher["Protocol"])
				publisher = publishers[2].(map[string]interface{})
				assert.Equal(t, 8080.0, publisher["TargetPort"])
				assert.Equal(t, 0.0, publisher["PublishedPort"])
				assert.Equal(t, "tcp", publisher["Protocol"])
				count++
			}
		}
		assert.Equal(t, 2, count, "Did not match both services:\n"+res.Combined())
	})
}
