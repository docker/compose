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

package compose

import (
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/google/go-cmp/cmp"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/internal"
	"github.com/docker/compose/v5/pkg/api"
)

func Test_createLayers(t *testing.T) {
	project, err := loader.LoadWithContext(t.Context(), types.ConfigDetails{
		WorkingDir:  "testdata/publish/",
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{
			{
				Filename: "testdata/publish/compose.yaml",
			},
		},
	})
	assert.NilError(t, err)
	project.ComposeFiles = []string{"testdata/publish/compose.yaml"}

	service := &composeService{}
	layers, err := service.createLayers(t.Context(), project, api.PublishOptions{
		WithEnvironment: true,
	})
	assert.NilError(t, err)

	published := string(layers[0].Data)
	assert.Equal(t, published, `name: test
services:
  test:
    extends:
      file: f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c.yaml
      service: foo

  string:
    image: test
    env_file: 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env

  list:
    image: test
    env_file:
      - 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env

  mapping:
    image: test
    env_file:
      - path: 5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3.env
`)

	expectedLayers := []v1.Descriptor{
		{
			MediaType: "application/vnd.docker.compose.file+yaml",
			Annotations: map[string]string{
				"com.docker.compose.file":    "compose.yaml",
				"com.docker.compose.version": internal.Version,
			},
		},
		{
			MediaType: "application/vnd.docker.compose.file+yaml",
			Annotations: map[string]string{
				"com.docker.compose.extends": "true",
				"com.docker.compose.file":    "f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c",
				"com.docker.compose.version": internal.Version,
			},
		},
		{
			MediaType: "application/vnd.docker.compose.envfile",
			Annotations: map[string]string{
				"com.docker.compose.envfile": "5efca9cdbac9f5394c6c2e2094b1b42661f988f57fcab165a0bf72b205451af3",
				"com.docker.compose.version": internal.Version,
			},
		},
	}
	assert.DeepEqual(t, expectedLayers, layers, cmp.FilterPath(func(path cmp.Path) bool {
		return !slices.Contains([]string{".Data", ".Digest", ".Size"}, path.String())
	}, cmp.Ignore()))
}
