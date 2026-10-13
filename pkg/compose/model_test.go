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
	"runtime"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

// TestEnsureModels_DryRunDoesNotPullNorConfigure: the docker model plugin is
// an external process the dry-run API client cannot intercept, so only its
// read-only commands (ls, status) may run in dry-run mode.
func TestEnsureModels_DryRunDoesNotPullNorConfigure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake plugin is a shell script")
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
case "$1" in
docker-cli-plugin-metadata) echo '{"SchemaVersion":"0.1.0","Vendor":"test","Version":"v1.2.6"}' ;;
ls) echo '[]' ;;
status) echo '{"endpoint":"http://model-runner.test"}' ;;
*) echo "$*" >> "` + calls + `" ;;
esac
`
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker-model"), []byte(script), 0o700))

	cli := mocks.NewMockCli(gomock.NewController(t))
	cli.EXPECT().ConfigFile().Return(&configfile.ConfigFile{CLIPluginsExtraDirs: []string{dir}}).AnyTimes()
	cli.EXPECT().CurrentContext().Return("default").AnyTimes()
	cli.EXPECT().DockerEndpoint().Return(docker.Endpoint{}).AnyTimes()
	s := &composeService{dockerCli: cli, dryRun: true, events: &ignore{}}

	// Environment and Models are promoted fields, which struct literals
	// cannot set before go1.27
	var app types.ServiceConfig
	app.Environment = types.MappingWithEquals{}
	app.Models = map[string]*types.ServiceModelConfig{"llm": nil}
	project := &types.Project{
		Models:   types.Models{"llm": {Model: "ai/smollm2"}},
		Services: types.Services{"app": app},
	}
	assert.NilError(t, s.ensureModels(t.Context(), project, false))

	_, err := os.Stat(calls)
	assert.Assert(t, os.IsNotExist(err), "pull and configure must not be executed in dry-run mode")
	url := project.Services["app"].Environment["LLM_URL"]
	assert.Assert(t, url != nil && *url == "http://model-runner.test", "model variables are still computed from the read-only status")
}
