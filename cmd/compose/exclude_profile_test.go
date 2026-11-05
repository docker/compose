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
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/pflag"
	"gotest.tools/v3/assert"

	realcompose "github.com/docker/compose/v5/pkg/compose"
)

func TestExcludeProfileFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	assert.NilError(t, os.WriteFile(path, []byte(`
services:
  web:
    image: nginx
  api:
    image: api
    profiles: [app]
  api-debug:
    image: api
    profiles: [app, debug]
  monitoring:
    image: prom
    profiles: [monitoring]
`), 0o644))

	opts := &ProjectOptions{}
	flags := pflag.NewFlagSet("compose", pflag.ContinueOnError)
	opts.addProjectFlags(flags)
	assert.NilError(t, flags.Parse([]string{
		"-f", path,
		"--profile", "*",
		"--exclude-profile", "debug",
		"--exclude-profile", "monitoring",
	}))

	backend, err := realcompose.NewComposeService(nil)
	assert.NilError(t, err)
	project, _, err := opts.ToProject(t.Context(), nil, backend, nil, skipUnsupportedAttributesWarning)
	assert.NilError(t, err)

	names := project.ServiceNames()
	slices.Sort(names)
	assert.DeepEqual(t, names, []string{"api", "web"})
}
