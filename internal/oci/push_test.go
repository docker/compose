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

package oci

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestDescriptors(t *testing.T) {
	compose := DescriptorForComposeFile("/some/dir/compose.yaml", []byte("services: {}"))
	assert.Equal(t, compose.MediaType, ComposeYAMLMediaType)
	assert.Equal(t, compose.Size, int64(len("services: {}")))
	assert.DeepEqual(t, compose.Annotations, map[string]string{
		"com.docker.compose.version": api.ComposeVersion,
		"com.docker.compose.file":    "compose.yaml",
	})

	env := DescriptorForEnvFile("/some/dir/.env", []byte("A=1"))
	assert.Equal(t, env.MediaType, ComposeEnvFileMediaType)
	assert.DeepEqual(t, env.Annotations, map[string]string{
		"com.docker.compose.version": api.ComposeVersion,
		"com.docker.compose.envfile": ".env",
	})
	assert.Equal(t, string(env.Data), "A=1")
}
