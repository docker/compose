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
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	logrustest "github.com/sirupsen/logrus/hooks/test"
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

// writeDotEnv writes the project's .env and makes sure the process starts
// without a COMPOSE_FILE of its own, restoring whatever it was afterwards:
// the root pre-run (setEnvWithDotEnv) copies the COMPOSE_* keys of .env into
// the process environment, and the test must not leak that into the others.
func writeDotEnv(t *testing.T, dir, content string) {
	t.Helper()
	t.Setenv("COMPOSE_FILE", "")
	assert.NilError(t, os.Unsetenv("COMPOSE_FILE"))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600))
}

// preRun does what the root command's PersistentPreRunE does before any
// command reads the environment: COMPOSE_* keys of .env become visible to
// os.Getenv, which is all projectOrName looks at.
func preRun(t *testing.T, opts ProjectOptions) {
	t.Helper()
	assert.NilError(t, setEnvWithDotEnv(opts, resolutionCli(t)))
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
		hook := logrustest.NewGlobal()

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "fallback")
		assert.Assert(t, project == nil)
		assert.Equal(t, len(hook.AllEntries()), 1)
		assert.Assert(t, strings.Contains(hook.LastEntry().Message, "falling back to label-based mode"), hook.LastEntry().Message)
	})

	// A file that exists but cannot be fully loaded because something it
	// references is missing is "present but broken", not "no compose file
	// around": the warning must fire even though the underlying error wraps
	// os.ErrNotExist.
	for name, content := range map[string]string{
		"include of a missing file":   "include:\n  - missing.yaml\nservices:\n  web:\n    image: alpine\n",
		"extends from a missing file": "services:\n  web:\n    extends:\n      file: nofile.yaml\n      service: x\n",
	} {
		t.Run("broken implicit file referencing a missing file warns: "+name, func(t *testing.T) {
			t.Setenv(ComposeProjectName, "fallback")
			dir := projectDir(t, content)
			opts := ProjectOptions{ProjectDir: dir}
			hook := logrustest.NewGlobal()

			project, resolved, err := opts.projectOrName(t.Context(), resolutionCli(t))
			assert.NilError(t, err)
			assert.Equal(t, resolved, "fallback")
			assert.Assert(t, project == nil)
			assert.Equal(t, len(hook.AllEntries()), 1, hook.AllEntries())
			assert.Assert(t, strings.Contains(hook.LastEntry().Message, "falling back to label-based mode"), hook.LastEntry().Message)
		})
	}

	// COMPOSE_FILE is the environment equivalent of --file: naming a file
	// that cannot be read is as much a hard error, never a silent fallback.
	t.Run("COMPOSE_FILE naming a missing file is a hard error even with COMPOSE_PROJECT_NAME", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "fallback")
		dir := projectDir(t, "")
		t.Setenv("COMPOSE_FILE", filepath.Join(dir, "nonexistent.yaml"))
		opts := ProjectOptions{ProjectDir: dir}

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.ErrorContains(t, err, "nonexistent.yaml")
	})

	t.Run("broken COMPOSE_FILE is a hard error with --project-name too", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "fallback")
		dir := projectDir(t, "services: {invalid")
		t.Setenv("COMPOSE_FILE", filepath.Join(dir, "compose.yaml"))
		opts := ProjectOptions{ProjectName: "explicit", ProjectDir: dir}

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.ErrorContains(t, err, "yaml")
	})

	t.Run("COMPOSE_FILE is loaded with --project-name", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, validCompose)
		t.Setenv("COMPOSE_FILE", filepath.Join(dir, "compose.yaml"))
		opts := ProjectOptions{ProjectName: "explicit", ProjectDir: dir}

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "explicit")
		assert.Assert(t, project != nil)
	})

	t.Run("COMPOSE_FILE set in .env is explicit too", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, "services: {invalid")
		writeDotEnv(t, dir, "COMPOSE_FILE="+filepath.Join(dir, "compose.yaml")+"\n")
		opts := ProjectOptions{ProjectName: "explicit", ProjectDir: dir}
		preRun(t, opts)

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.ErrorContains(t, err, "yaml")
	})

	t.Run("COMPOSE_FILE in .env naming a missing file is a hard error with --project-name", func(t *testing.T) {
		unsetEnv(t)
		dir := projectDir(t, "")
		writeDotEnv(t, dir, "COMPOSE_FILE="+filepath.Join(dir, "nonexistent.yaml")+"\n")
		opts := ProjectOptions{ProjectName: "explicit", ProjectDir: dir}
		preRun(t, opts)

		_, _, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.ErrorContains(t, err, "nonexistent.yaml")
	})

	t.Run("nonexistent --project-directory with COMPOSE_PROJECT_NAME stays silent", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "labels-only")
		opts := ProjectOptions{ProjectDir: filepath.Join(t.TempDir(), "does-not-exist")}
		hook := logrustest.NewGlobal()

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "labels-only")
		assert.Assert(t, project == nil)
		assert.Equal(t, len(hook.AllEntries()), 0, hook.AllEntries())
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
		hook := logrustest.NewGlobal()

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t))
		assert.NilError(t, err)
		assert.Equal(t, name, "labels-only")
		assert.Assert(t, project == nil)
		// no file exists at all, so the "found but could not be loaded"
		// warning would be factually wrong here — this is the normal
		// file-less workflow, not a fallback from a broken file.
		assert.Equal(t, len(hook.AllEntries()), 0, hook.AllEntries())
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

	// The file loads fine, only the selection fails: the label-based fallback
	// is what lets `stop` reach the container of a service removed from the
	// file, but saying the file "could not be loaded" would be false.
	// An explicit file that loads is no different: the selection failure is
	// not an unreadable file, and the container of a service removed from it
	// must stay reachable. -p keeps its precedence over COMPOSE_PROJECT_NAME.
	for _, tc := range []struct {
		name        string
		envName     string
		projectName string
		viaFlag     bool
		viaDotEnv   bool
		want        string
	}{
		{name: "COMPOSE_FILE", envName: "fallback", want: "fallback"},
		{name: "--file", envName: "fallback", viaFlag: true, want: "fallback"},
		{name: "COMPOSE_FILE set in .env", envName: "fallback", viaDotEnv: true, want: "fallback"},
		{name: "COMPOSE_FILE and --project-name only", projectName: "flag", want: "flag"},
		{name: "COMPOSE_FILE, --project-name and COMPOSE_PROJECT_NAME", envName: "env", projectName: "flag", want: "flag"},
	} {
		t.Run("unknown requested service with an explicit file that loads falls back silently: "+tc.name, func(t *testing.T) {
			if tc.envName != "" {
				t.Setenv(ComposeProjectName, tc.envName)
			} else {
				unsetEnv(t)
			}
			dir := projectDir(t, validCompose)
			file := filepath.Join(dir, "compose.yaml")
			opts := ProjectOptions{ProjectDir: dir, ProjectName: tc.projectName}
			switch {
			case tc.viaFlag:
				opts.ConfigPaths = []string{file}
			case tc.viaDotEnv:
				writeDotEnv(t, dir, "COMPOSE_FILE="+file+"\n")
				preRun(t, opts)
			default:
				t.Setenv("COMPOSE_FILE", file)
			}
			hook := logrustest.NewGlobal()

			project, name, err := opts.projectOrName(t.Context(), resolutionCli(t), "typo")
			assert.NilError(t, err)
			assert.Equal(t, name, tc.want)
			assert.Assert(t, project == nil)
			assert.Equal(t, len(hook.AllEntries()), 0, hook.AllEntries())
		})
	}

	// --project-name with --file: the services are those of the file, and a
	// name it does not declare stays an error whatever COMPOSE_PROJECT_NAME
	// says, as it always was (it must not silently target another project).
	for name, env := range map[string]string{"without COMPOSE_PROJECT_NAME": "", "with COMPOSE_PROJECT_NAME": "env"} {
		t.Run("unknown requested service with --file and --project-name is an error "+name, func(t *testing.T) {
			if env != "" {
				t.Setenv(ComposeProjectName, env)
			} else {
				unsetEnv(t)
			}
			dir := projectDir(t, validCompose)
			opts := ProjectOptions{ProjectDir: dir, ProjectName: "flag", ConfigPaths: []string{filepath.Join(dir, "compose.yaml")}}

			_, _, err := opts.projectOrName(t.Context(), resolutionCli(t), "typo")
			assert.ErrorContains(t, err, "no such service: typo")
		})
	}

	t.Run("unknown requested service with COMPOSE_PROJECT_NAME falls back without a warning", func(t *testing.T) {
		t.Setenv(ComposeProjectName, "fallback")
		dir := projectDir(t, validCompose)
		opts := ProjectOptions{ProjectDir: dir}
		hook := logrustest.NewGlobal()

		project, name, err := opts.projectOrName(t.Context(), resolutionCli(t), "typo")
		assert.NilError(t, err)
		assert.Equal(t, name, "fallback")
		assert.Assert(t, project == nil)
		assert.Equal(t, len(hook.AllEntries()), 0, hook.AllEntries())
	})
}

// validateServiceNames backs the commands that don't pass their service
// arguments through the load-time selection (restart, wait): strict when a
// model is available, no-op in label-based mode.
func TestValidateServiceNames(t *testing.T) {
	project := &types.Project{
		Services:         types.Services{"web": {Name: "web"}},
		DisabledServices: types.Services{"gated": {Name: "gated"}},
		Jobs:             types.Jobs{"migrate": {Name: "migrate"}},
	}

	assert.NilError(t, validateServiceNames(nil, []string{"anything"}))
	assert.NilError(t, validateServiceNames(project, []string{"web"}))
	// profile-disabled services are legitimate targets: restart enables them
	assert.NilError(t, validateServiceNames(project, []string{"gated"}))
	assert.Error(t, validateServiceNames(project, []string{"typo"}), "no such service: typo")
	// compose-go selects services with filepath.Match: a pattern is not a
	// name to look up. Whether it matches something is for the selection that
	// resolves it to tell (restart rejects a pattern matching nothing).
	assert.NilError(t, validateServiceNames(project, []string{"we*"}))
	assert.NilError(t, validateServiceNames(project, []string{"w?b"}))
	assert.NilError(t, validateServiceNames(project, []string{"[wg]*"}))
	assert.Error(t, validateServiceNames(project, []string{"web", "typo"}), "no such service: typo")
	// restart/wait pass no services to projectOrName, so a job target never
	// reaches jobTargetErr there -- validateServiceNames must recognize it
	// itself instead of falling through to the generic "no such service".
	assert.Error(t, validateServiceNames(project, []string{"migrate"}), `job "migrate" can only be triggered with "docker compose run"`)
}
