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
	"context"
	"os"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/internal/ocipush"
	"github.com/docker/compose/v2/pkg/api"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

func Test_processExtends(t *testing.T) {
	project, err := loader.LoadWithContext(context.TODO(), types.ConfigDetails{
		WorkingDir:  "testdata/publish/",
		Environment: types.Mapping{},
		ConfigFiles: []types.ConfigFile{
			{
				Filename: "testdata/publish/compose.yaml",
			},
		},
	})
	assert.NilError(t, err)
	extFiles := map[string]string{}
	file, err := processFile(context.TODO(), "testdata/publish/compose.yaml", project, extFiles)
	assert.NilError(t, err)

	v := string(file)
	assert.Equal(t, v, `name: test
services:
  test:
    extends:
      file: f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c.yaml
      service: foo
`)

	layers, err := processExtends(context.TODO(), project, extFiles)
	assert.NilError(t, err)

	b, err := os.ReadFile("testdata/publish/common.yaml")
	assert.NilError(t, err)
	assert.DeepEqual(t, []ocipush.Pushable{
		{
			Descriptor: v1.Descriptor{
				MediaType: "application/vnd.docker.compose.file+yaml",
				Digest:    "sha256:d3ba84507b56ec783f4b6d24306b99a15285f0a23a835f0b668c2dbf9c59c241",
				Size:      32,
				Annotations: map[string]string{
					"com.docker.compose.extends": "true",
					"com.docker.compose.file":    "f8f9ede3d201ec37d5a5e3a77bbadab79af26035e53135e19571f50d541d390c.yaml",
					"com.docker.compose.version": api.ComposeVersion,
				},
			},
			Data: b,
		},
	}, layers)
}
