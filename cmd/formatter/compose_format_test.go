/*
   Copyright 2026 Docker Compose CLI authors

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

package formatter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/containerd/platforms"
	cliformatter "github.com/docker/cli/cli/command/formatter"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestProjectWriteCustomTemplate(t *testing.T) {
	projects := []Project{
		{Name: "alpha", Status: "running(1)", ConfigFiles: "compose.yaml"},
		{Name: "beta", Status: "exited(1)", ConfigFiles: "compose.yaml"},
	}

	var out bytes.Buffer
	err := ProjectWrite(cliformatter.Context{
		Output: &out,
		Format: NewProjectFormat("{{.Name}}"),
	}, projects)

	assert.NilError(t, err)
	assert.Equal(t, out.String(), "alpha\nbeta\n")
}

func TestProjectWriteJSONTemplate(t *testing.T) {
	projects := []Project{{Name: "alpha", Status: "running(1)", ConfigFiles: "compose.yaml"}}

	var out bytes.Buffer
	err := ProjectWrite(cliformatter.Context{
		Output: &out,
		Format: NewProjectFormat("{{json .}}"),
	}, projects)

	assert.NilError(t, err)
	var rendered map[string]any
	assert.NilError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &rendered))
	assert.Equal(t, rendered["Name"], "alpha")
	assert.Equal(t, rendered["Status"], "running(1)")
	assert.Equal(t, rendered["ConfigFiles"], "compose.yaml")
}

func TestImageWriteCustomTemplate(t *testing.T) {
	created := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	images := []Image{
		{
			ContainerName: "web-1",
			Summary: api.ImageSummary{
				ID:         "sha256:1234567890abcdef",
				Repository: "example/web",
				Tag:        "latest",
				Platform:   platforms.MustParse("linux/amd64"),
				Size:       239840,
				Created:    &created,
			},
		},
	}

	var out bytes.Buffer
	err := ImageWrite(cliformatter.Context{
		Output: &out,
		Format: NewImageFormat("{{.ContainerName}} {{.Repository}}:{{.Tag}} {{.Platform}}"),
	}, images)

	assert.NilError(t, err)
	assert.Equal(t, out.String(), "web-1 example/web:latest linux/amd64\n")
}

func TestImageWriteJSONTemplate(t *testing.T) {
	images := []Image{
		{
			ContainerName: "web-1",
			Summary: api.ImageSummary{
				ID:         "sha256:1234567890abcdef",
				Repository: "example/web",
				Tag:        "latest",
				Platform:   platforms.MustParse("linux/amd64"),
				Size:       239840,
			},
		},
	}

	var out bytes.Buffer
	err := ImageWrite(cliformatter.Context{
		Output: &out,
		Format: NewImageFormat("{{json .}}"),
	}, images)

	assert.NilError(t, err)
	var rendered map[string]any
	assert.NilError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &rendered))
	assert.Equal(t, rendered["ContainerName"], "web-1")
	assert.Equal(t, rendered["Repository"], "example/web")
	assert.Equal(t, rendered["Tag"], "latest")
	assert.Equal(t, rendered["Platform"], "linux/amd64")
}
