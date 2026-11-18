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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/mocks"
)

func removeOrphansFlagSet(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var v bool
	flags.BoolVar(&v, "remove-orphans", false, "")
	assert.NilError(t, flags.Parse(args))
	return flags
}

// An explicit --remove-orphans flag always wins; without it the value comes
// from COMPOSE_REMOVE_ORPHANS in the process environment — which the root
// PersistentPreRunE completed with the project's local .env beforehand.
func TestRemoveOrphansFromEnv(t *testing.T) {
	t.Run("explicit flag wins over the environment", func(t *testing.T) {
		t.Setenv(ComposeRemoveOrphans, "true")
		flags := removeOrphansFlagSet(t, "--remove-orphans=false")
		assert.Equal(t, removeOrphansFromEnv(flags, false), false)
	})

	t.Run("environment applies when the flag is not passed", func(t *testing.T) {
		t.Setenv(ComposeRemoveOrphans, "true")
		flags := removeOrphansFlagSet(t)
		assert.Equal(t, removeOrphansFromEnv(flags, false), true)
	})

	t.Run("defaults to false when neither is set", func(t *testing.T) {
		t.Setenv(ComposeRemoveOrphans, "")
		assert.NilError(t, os.Unsetenv(ComposeRemoveOrphans))
		flags := removeOrphansFlagSet(t)
		assert.Equal(t, removeOrphansFromEnv(flags, false), false)
	})
}

func writeProjectWithDotEnv(t *testing.T, dotEnv string) ProjectOptions {
	t.Helper()
	dir := t.TempDir()
	composePath := filepath.Join(dir, "compose.yaml")
	assert.NilError(t, os.WriteFile(composePath, []byte("services: {}\n"), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(dotEnv), 0o600))
	return ProjectOptions{
		ConfigPaths: []string{composePath},
		ProjectDir:  dir,
	}
}

// setEnvWithDotEnv turns the COMPOSE_* keys of the project's local .env into
// per-project defaults by completing the process environment — process
// values win, non-COMPOSE_ keys are left alone, and remote configs are
// deliberately excluded (COMPOSE_* variables are the local user's choice; a
// remote model must not steer the CLI consuming it).
func TestSetEnvWithDotEnv(t *testing.T) {
	unset := func(t *testing.T, keys ...string) {
		t.Helper()
		for _, k := range keys {
			t.Setenv(k, "") // registers restoration
			assert.NilError(t, os.Unsetenv(k))
		}
	}

	t.Run("COMPOSE_ keys of the local .env are injected", func(t *testing.T) {
		unset(t, ComposeRemoveOrphans, "NOT_COMPOSE_VAR")
		opts := writeProjectWithDotEnv(t, "COMPOSE_REMOVE_ORPHANS=true\nNOT_COMPOSE_VAR=x\n")

		assert.NilError(t, setEnvWithDotEnv(opts, nil))

		assert.Equal(t, os.Getenv(ComposeRemoveOrphans), "true")
		_, injected := os.LookupEnv("NOT_COMPOSE_VAR")
		assert.Check(t, !injected, "non-COMPOSE_ keys must not leak into the process environment")
	})

	t.Run("process environment wins over the .env", func(t *testing.T) {
		t.Setenv(ComposeRemoveOrphans, "false")
		opts := writeProjectWithDotEnv(t, "COMPOSE_REMOVE_ORPHANS=true\n")

		assert.NilError(t, setEnvWithDotEnv(opts, nil))

		assert.Equal(t, os.Getenv(ComposeRemoveOrphans), "false")
	})

	t.Run("remote configs are excluded", func(t *testing.T) {
		unset(t, ComposeRemoveOrphans)
		opts := writeProjectWithDotEnv(t, "COMPOSE_REMOVE_ORPHANS=true\n")
		opts.ConfigPaths = []string{"test://remote/compose.yaml"}
		opts.remoteLoadersOverride = []loader.ResourceLoader{testRemoteLoader{}}

		assert.NilError(t, setEnvWithDotEnv(opts, nil))

		_, injected := os.LookupEnv(ComposeRemoveOrphans)
		assert.Check(t, !injected, "a remote model must not inject COMPOSE_* variables")
	})
}

// G.3 of epic #14074: the full resolution order, .env → process env → flag.
func TestRemoveOrphansResolutionOrder(t *testing.T) {
	t.Setenv(ComposeRemoveOrphans, "")
	assert.NilError(t, os.Unsetenv(ComposeRemoveOrphans))
	opts := writeProjectWithDotEnv(t, "COMPOSE_REMOVE_ORPHANS=true\n")

	// .env applies when nothing else is set
	assert.NilError(t, setEnvWithDotEnv(opts, nil))
	assert.Equal(t, removeOrphansFromEnv(removeOrphansFlagSet(t), false), true)

	// an explicit flag beats both
	assert.Equal(t, removeOrphansFromEnv(removeOrphansFlagSet(t, "--remove-orphans=false"), false), false)
}

// Every command carrying --remove-orphans resolves COMPOSE_REMOVE_ORPHANS
// through its own PreRunE: the real commands are run with RunE stubbed out.
func TestRemoveOrphansIsResolvedByEveryCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	dockerCli := mocks.NewMockCli(ctrl)
	dockerCli.EXPECT().In().Return(streams.NewIn(io.NopCloser(strings.NewReader("")))).AnyTimes()
	dockerCli.EXPECT().Out().Return(streams.NewOut(io.Discard)).AnyTimes()
	dockerCli.EXPECT().Err().Return(streams.NewOut(io.Discard)).AnyTimes()

	commands := map[string]func() *cobra.Command{
		"create": func() *cobra.Command { return createCommand(&ProjectOptions{}, dockerCli, &BackendOptions{}) },
		"down":   func() *cobra.Command { return downCommand(&ProjectOptions{}, dockerCli, &BackendOptions{}) },
		"kill":   func() *cobra.Command { return killCommand(&ProjectOptions{}, dockerCli, &BackendOptions{}) },
		"run":    func() *cobra.Command { return runCommand(&ProjectOptions{}, dockerCli, &BackendOptions{}) },
		"up":     func() *cobra.Command { return upCommand(&ProjectOptions{}, dockerCli, &BackendOptions{}) },
	}
	tests := []struct {
		name string
		env  string
		args []string
		want string
	}{
		{name: "environment applies", env: "true", want: "true"},
		{name: "explicit flag", args: []string{"--remove-orphans"}, want: "true"},
		{name: "explicit flag beats the environment", env: "true", args: []string{"--remove-orphans=false"}, want: "false"},
		{name: "unset", want: "false"},
	}
	for name, newCmd := range commands {
		for _, tt := range tests {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				t.Setenv(ComposeRemoveOrphans, tt.env)
				cmd := newCmd()
				cmd.RunE = func(*cobra.Command, []string) error { return nil }
				// run takes a mandatory service argument
				cmd.SetArgs(append(tt.args, "svc"))

				assert.NilError(t, cmd.ExecuteContext(t.Context()))

				assert.Equal(t, cmd.Flags().Lookup("remove-orphans").Value.String(), tt.want)
			})
		}
	}
}

// COMPOSE_IGNORE_ORPHANS comes from the project environment and conflicts
// with --remove-orphans, whichever command resolves it.
func TestResolveIgnoreOrphans(t *testing.T) {
	project := func(value string) *types.Project {
		return &types.Project{Environment: types.Mapping{ComposeIgnoreOrphans: value}}
	}

	ignore, err := resolveIgnoreOrphans(project("true"), false)
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)

	ignore, err = resolveIgnoreOrphans(project(""), true)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)

	_, err = resolveIgnoreOrphans(project("true"), true)
	assert.ErrorContains(t, err, "cannot combine COMPOSE_IGNORE_ORPHANS and --remove-orphans")
}
