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

package formatter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/docker/cli/cli/command/formatter"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestContainerContextEngine(t *testing.T) {
	withLabel := ContainerContext{c: api.ContainerSummary{
		Labels: map[string]string{api.ContainerEngineLabel: "moby"},
	}}
	assert.Equal(t, withLabel.Engine(), "moby")

	noLabels := ContainerContext{c: api.ContainerSummary{}}
	assert.Equal(t, noLabels.Engine(), "")

	otherLabel := ContainerContext{c: api.ContainerSummary{
		Labels: map[string]string{api.ProjectLabel: "test"},
	}}
	assert.Equal(t, otherLabel.Engine(), "")
}

func TestContainerWriteEngineColumn(t *testing.T) {
	containers := []api.ContainerSummary{
		{
			Name:    "with-engine",
			Service: "svc1",
			Labels:  map[string]string{api.ContainerEngineLabel: "moby"},
		},
		{
			Name:    "without-engine",
			Service: "svc2",
		},
	}

	t.Run("engine column shown when label present", func(t *testing.T) {
		var out bytes.Buffer
		ctx := formatter.Context{
			Output: &out,
			Format: NewContainerFormat("table", false, false, true),
		}
		assert.NilError(t, ContainerWrite(ctx, containers))
		assert.Assert(t, strings.Contains(out.String(), engineHeader), out.String())
		assert.Assert(t, strings.Contains(out.String(), "moby"), out.String())
	})

	t.Run("engine column hidden when no label present", func(t *testing.T) {
		var out bytes.Buffer
		ctx := formatter.Context{
			Output: &out,
			Format: NewContainerFormat("table", false, false, false),
		}
		assert.NilError(t, ContainerWrite(ctx, containers))
		assert.Assert(t, !strings.Contains(out.String(), engineHeader), out.String())
	})
}
