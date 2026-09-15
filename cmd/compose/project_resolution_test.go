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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func projectDir(t *testing.T, composeContent string) string {
	t.Helper()
	dir := t.TempDir()
	if composeContent != "" {
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeContent), 0o600))
	}
	return dir
}

func resolutionCli(t *testing.T) *mocks.MockCli {
	t.Helper()
	ctrl := gomock.NewController(t)
	cli := mocks.NewMockCli(ctrl)
	cli.EXPECT().Out().Return(streams.NewOut(os.Stdout)).AnyTimes()
	cli.EXPECT().Err().Return(streams.NewOut(os.Stderr)).AnyTimes()
	return cli
}

const validCompose = "services:\n  web:\n    image: alpine\n  gated:\n    image: alpine\n    profiles: [debug]\n"

// The resolution matrix of projectOrName: one name precedence
// (--project-name, then COMPOSE_PROJECT_NAME, then the model), a hard error
// for an explicit --file that cannot be read, and a label-based fallback
// only when COMPOSE_PROJECT_NAME provides a name.
func TestProjectOrNameResolution(t *testing.T) {
	unsetEnv := func(t *testing.T) {
		t.Setenv(ComposeProjectName, "")
		assert.NilError(t, os.Unsetenv(ComposeProjectName))
	}

	t.Run("broken implicit file falls back to COMPOSE_PROJECT_NAME", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "fallback")
		dir := projectDir(t, "services: {invalid")
		opts := ProjectOptions{ProjectDir: dir}

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "fallback")
		assert.Assert(t, project == nil)
	})

	t.Run("broken explicit --file is a hard error even with COMPOSE_PROJECT_NAME", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "fallback")
		dir := projectDir(t, "services: {invalid")
		opts := ProjectOptions{ConfigPaths: []string{filepath.Join(dir, "compose.yaml")}, ProjectDir: dir}

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.ErrorContains(t, err, "yaml")
	})

	t.Run("no file at all with COMPOSE_PROJECT_NAME is the file-less workflow", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "labels-only")
		dir := projectDir(t, "")
		opts := ProjectOptions{ProjectDir: dir}

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "labels-only")
		assert.Assert(t, project == nil)
	})

	t.Run("no file and no name errors", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, "")
		opts := ProjectOptions{ProjectDir: dir}

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.Assert(t, err != nil)
	})

	t.Run("COMPOSE_PROJECT_NAME overrides the loaded model's name", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "from-env")
		dir := projectDir(t, validCompose)
		opts := ProjectOptions{ProjectDir: dir}

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "from-env")
		assert.Assert(t, project != nil)
	})

	t.Run("--project-name without --file skips loading, even a broken file", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, "services: {invalid")
		opts := ProjectOptions{ProjectName: "explicit", ProjectDir: dir}

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "explicit")
		assert.Assert(t, project == nil)
	})

	t.Run("unknown requested service is rejected by the load itself", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, validCompose)
		opts := ProjectOptions{ProjectDir: dir}

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t), "typo")
		assert.ErrorContains(t, err, "no such service")
	})
}

// validateServiceNames backs the commands that don't pass their service
// arguments through the load-time selection (restart, wait): strict when a
// model is available, no-op in label-based mode.
func TestValidateServiceNames(t *testing.T) {
	project := &types.Project{
		Services:         types.Services{"web": {Name: "web"}},
		DisabledServices: types.Services{"gated": {Name: "gated"}},
	}

	assert.NilError(t, validateServiceNames(nil, []string{"anything"}))
	assert.NilError(t, validateServiceNames(project, []string{"web"}))
	// profile-disabled services are legitimate targets: restart enables them
	assert.NilError(t, validateServiceNames(project, []string{"gated"}))
	assert.Error(t, validateServiceNames(project, []string{"typo"}), "no such service: typo")
}
