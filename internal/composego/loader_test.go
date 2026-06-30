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

package composego

import (
	"os"
	"path/filepath"
	"testing"

	composeloader "github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
)

func TestExtendsResetDoesNotLeakOverriddenNetworkAliases(t *testing.T) {
	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")
	composeContent := `
services:
  service_a:
    image: php:latest
    networks:
      default:
        aliases:
          - oss-a.xyz
  service_b:
    extends:
      service: service_a
    networks: !override
      default:
        aliases:
          - oss-b.xyz
  service_c:
    extends:
      service: service_b
    networks: !reset []
`
	err := os.WriteFile(composeFile, []byte(composeContent), 0o644)
	assert.NilError(t, err)

	project, err := composeloader.LoadWithContext(t.Context(), composetypes.ConfigDetails{
		WorkingDir:  tmpDir,
		Environment: composetypes.Mapping{},
		ConfigFiles: []composetypes.ConfigFile{{Filename: composeFile}},
	}, func(options *composeloader.Options) {
		options.SetProjectName("compose-13348", true)
	})
	assert.NilError(t, err)

	// Regression test for docker/compose#13348: the chained reset must not mutate
	// service_b and reintroduce service_a's aliases after service_b used !override.
	serviceB := project.Services["service_b"]
	assert.DeepEqual(t, []string{"oss-b.xyz"}, serviceB.Networks["default"].Aliases)
}
