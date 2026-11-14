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
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	cliopts "github.com/docker/cli/opts"
	"github.com/spf13/cobra"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/mocks"
)

func TestParseServicesReplicasArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    map[string]int
		wantErr string
	}{
		{name: "no argument", want: map[string]int{}},
		{name: "several services", args: []string{"web=2", "db=0"}, want: map[string]int{"web": 2, "db": 0}},
		{name: "last value wins", args: []string{"web=2", "web=5"}, want: map[string]int{"web": 5}},
		{name: "missing separator", args: []string{"web"}, wantErr: "invalid scale specifier: web"},
		{name: "missing service", args: []string{"=2"}, wantErr: "invalid scale specifier: =2"},
		{name: "missing value", args: []string{"web="}, wantErr: "invalid scale specifier: web="},
		{name: "not a number", args: []string{"web=two"}, wantErr: "invalid scale specifier: can't parse replica value as int: web=two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServicesReplicasArgs(tt.args)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestPsParseFilter(t *testing.T) {
	tests := []struct {
		name       string
		filter     string
		status     []string
		wantStatus []string
		wantErr    string
	}{
		{name: "no filter"},
		{name: "status", filter: "status=running", wantStatus: []string{"running"}},
		{name: "status is appended", filter: "status=exited", status: []string{"running"}, wantStatus: []string{"running", "exited"}},
		{name: "not a key value pair", filter: "running", wantErr: "arguments to --filter should be in form KEY=VAL"},
		{name: "source is not implemented", filter: "source=build", wantErr: api.ErrNotImplemented.Error()},
		{name: "unknown key", filter: "health=healthy", wantErr: "unknown filter health"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := psOptions{Filter: tt.filter, Status: tt.status}
			err := opts.parseFilter()
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, opts.Status, tt.wantStatus)
		})
	}
}

func TestFilterByStatus(t *testing.T) {
	containers := []api.ContainerSummary{
		{Name: "a", State: "running"},
		{Name: "b", State: "exited"},
		{Name: "c", State: "paused"},
		{Name: "d", State: "running"},
	}
	names := func(in []api.ContainerSummary) []string {
		var out []string
		for _, c := range in {
			out = append(out, c.Name)
		}
		return out
	}

	tests := []struct {
		name     string
		statuses []string
		want     []string
	}{
		{name: "single status", statuses: []string{"running"}, want: []string{"a", "d"}},
		{name: "several statuses", statuses: []string{"exited", "paused"}, want: []string{"b", "c"}},
		{name: "no match", statuses: []string{"restarting"}},
		{name: "no status selects nothing", statuses: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, names(filterByStatus(containers, tt.statuses)), tt.want)
		})
	}
}

func TestNormalizeRunFlags(t *testing.T) {
	tests := []struct{ in, want string }{
		{"volumes", "volume"},
		{"labels", "label"},
		{"no-TTY", "no-tty"},
		{"user", "user"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, string(normalizeRunFlags(nil, tt.in)), tt.want)
		})
	}
}

func TestRunOptionsApply(t *testing.T) {
	project := func() *types.Project {
		return &types.Project{
			Services: types.Services{
				"web": {
					Name: "web",
					WorkloadSpec: types.WorkloadSpec{
						Ports:     []types.ServicePortConfig{{Target: 80, Published: "8080"}},
						DependsOn: types.DependsOnConfig{"db": {Condition: types.ServiceConditionStarted, Required: true}},
					},
				},
				"db": {Name: "db"},
			},
		}
	}

	tests := []struct {
		name      string
		opts      runOptions
		wantErr   string
		check     func(t *testing.T, p *types.Project)
		wantNames []string
	}{
		{
			name:      "tty and no stdin by default",
			opts:      runOptions{Service: "web", interactive: false},
			wantNames: []string{"db", "web"},
			check: func(t *testing.T, p *types.Project) {
				assert.Assert(t, p.Services["web"].Tty)
				assert.Assert(t, !p.Services["web"].StdinOpen)
			},
		},
		{
			name: "no-tty and interactive",
			opts: runOptions{Service: "web", noTty: true, interactive: true},
			check: func(t *testing.T, p *types.Project) {
				assert.Assert(t, !p.Services["web"].Tty)
				assert.Assert(t, p.Services["web"].StdinOpen)
			},
		},
		{
			name: "service ports are dropped unless requested",
			opts: runOptions{Service: "web"},
			check: func(t *testing.T, p *types.Project) {
				assert.Equal(t, len(p.Services["web"].Ports), 0)
			},
		},
		{
			name: "service-ports keeps the declared ports",
			opts: runOptions{Service: "web", servicePorts: true},
			check: func(t *testing.T, p *types.Project) {
				assert.Equal(t, len(p.Services["web"].Ports), 1)
			},
		},
		{
			name: "publish replaces the declared ports",
			opts: runOptions{Service: "web", publish: []string{"9090:90"}},
			check: func(t *testing.T, p *types.Project) {
				ports := p.Services["web"].Ports
				assert.Equal(t, len(ports), 1)
				assert.Equal(t, ports[0].Published, "9090")
				assert.Equal(t, ports[0].Target, uint32(90))
			},
		},
		{
			name: "publish is ignored with service-ports",
			opts: runOptions{Service: "web", servicePorts: true, publish: []string{"9090:90"}},
			check: func(t *testing.T, p *types.Project) {
				assert.Equal(t, p.Services["web"].Ports[0].Published, "8080")
			},
		},
		{
			name:      "no-deps selects the target only",
			opts:      runOptions{Service: "web", noDeps: true},
			wantNames: []string{"web"},
		},
		{
			name: "volume is added to the service",
			opts: runOptions{Service: "web", volumes: []string{"/host:/data"}},
			check: func(t *testing.T, p *types.Project) {
				volumes := p.Services["web"].Volumes
				assert.Equal(t, len(volumes), 1)
				assert.Equal(t, volumes[0].Source, "/host")
				assert.Equal(t, volumes[0].Target, "/data")
			},
		},
		{name: "invalid publish", opts: runOptions{Service: "web", publish: []string{"not-a-port"}}, wantErr: "invalid containerPort: not-a-port"},
		{name: "invalid volume", opts: runOptions{Service: "web", volumes: []string{"/a:/b:rw:x"}}, wantErr: "invalid spec: /a:/b:rw:x: too many colons"},
		{name: "unknown service", opts: runOptions{Service: "nope"}, wantErr: "no such service: nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.opts.apply(project())
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			if tt.wantNames != nil {
				assert.DeepEqual(t, got.ServiceNames(), tt.wantNames)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToAPIBuildOptionsSSH(t *testing.T) {
	tests := []struct {
		name    string
		ssh     string
		want    []types.SSHKey
		wantErr string
	}{
		{name: "no ssh"},
		{name: "default agent", ssh: "default", want: []types.SSHKey{{ID: "default"}}},
		{name: "id and path", ssh: "gh=/home/me/.ssh/id", want: []types.SSHKey{{ID: "gh", Path: "/home/me/.ssh/id"}}},
		{name: "id without path", ssh: "gh", wantErr: `invalid ssh key "gh"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildOptions{ProjectOptions: &ProjectOptions{}, ssh: tt.ssh}
			got, err := opts.toAPIBuildOptions(nil)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got.SSHs, tt.want)
		})
	}
}

func TestToAPIBuildOptionsMapping(t *testing.T) {
	t.Setenv("BUILDX_BUILDER", "from-env")

	opts := buildOptions{
		ProjectOptions: &ProjectOptions{},
		pull:           true,
		quiet:          true,
		print:          true,
		memory:         cliopts.MemBytes(512 * 1024 * 1024),
		args:           []string{"FOO=bar", "BAZ"},
		sbom:           "true",
		provenance:     "mode=max",
	}
	got, err := opts.toAPIBuildOptions([]string{"web"})
	assert.NilError(t, err)
	assert.DeepEqual(t, got.Services, []string{"web"})
	assert.Equal(t, got.Pull, true)
	assert.Equal(t, got.Push, false)
	assert.Equal(t, got.NoCache, false)
	assert.Equal(t, got.Quiet, true)
	assert.Equal(t, got.Deps, false)
	assert.Equal(t, got.Print, true)
	assert.Equal(t, got.Check, false)
	assert.Equal(t, got.Memory, int64(512*1024*1024))
	assert.Equal(t, got.SBOM, "true")
	assert.Equal(t, got.Provenance, "mode=max")
	assert.Equal(t, got.Builder, "from-env", "BUILDX_BUILDER is the fallback when --builder is not set")
	assert.Equal(t, *got.Args["FOO"], "bar")
	assert.Assert(t, got.Args["BAZ"] == nil, "a build arg without value is resolved later from the environment")

	opts.builder = "from-flag"
	got, err = opts.toAPIBuildOptions(nil)
	assert.NilError(t, err)
	assert.Equal(t, got.Builder, "from-flag", "--builder wins over BUILDX_BUILDER")
}

// commandWithFlags returns a cobra command whose flags were parsed from args
func commandWithFlags(t *testing.T, name string, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: name}
	cmd.Flags().String("ansi", "auto", "")
	cmd.Flags().Int("parallel", -1, "")
	assert.NilError(t, cmd.ParseFlags(args))
	return cmd
}

func ptr(s string) *string { return &s }

func TestResolveAnsiMode(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		ansi    string
		noAnsi  bool
		env     *string
		want    string
		wantErr string
	}{
		{name: "default", ansi: "auto", want: "auto"},
		{name: "deprecated no-ansi", ansi: "auto", noAnsi: true, want: "never"},
		{
			name: "no-ansi with explicit ansi", ansi: "always", noAnsi: true, args: []string{"--ansi", "always"},
			wantErr: `cannot specify DEPRECATED "--no-ansi" and "--ansi". Please use only "--ansi"`,
		},
		{name: "environment applies without flag", ansi: "auto", env: ptr("never"), want: "never"},
		{name: "environment overrides no-ansi", ansi: "auto", noAnsi: true, env: ptr("always"), want: "always"},
		{name: "flag wins over environment", ansi: "never", args: []string{"--ansi", "never"}, env: ptr("always"), want: "never"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == nil {
				unsetEnv(t, "COMPOSE_ANSI")
			} else {
				t.Setenv("COMPOSE_ANSI", *tt.env)
			}
			root := commandWithFlags(t, PluginName, tt.args...)
			sub := &cobra.Command{Use: "ps"}
			root.AddCommand(sub)
			got, err := resolveAnsiMode(sub, tt.ansi, tt.noAnsi)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}

	// Intent: the --ansi flag lives on the compose command, so a command
	// outside of its tree can't tell whether the flag was set.
	t.Run("command outside of compose", func(t *testing.T) {
		_, err := resolveAnsiMode(&cobra.Command{Use: "other"}, "auto", false)
		assert.Error(t, err, `error parsing command line, expected "compose"`)
	})
}

// TestAnsiPrecedenceThroughRootCommand runs the real command tree, where --ansi
// is registered on the compose command only while the pre-run hook receives
// the executed sub-command. The resolved mode is observed through the
// --progress/--ansi compatibility check of selectEventProcessor. Every case
// resolves to "always" on purpose: any other mode would switch ANSI off for
// the rest of the test binary (formatter keeps that in a package global).
func TestAnsiPrecedenceThroughRootCommand(t *testing.T) {
	const forced = "can't use --progress plain while ANSI support is forced"
	tests := []struct {
		name    string
		args    []string
		env     string
		wantErr string
	}{
		{
			name:    "flag wins over environment",
			args:    []string{"--ansi", "always", "--progress", "plain", "ps"},
			env:     "never",
			wantErr: forced,
		},
		{
			name:    "environment applies without flag",
			args:    []string{"--progress", "plain", "ps"},
			env:     "always",
			wantErr: forced,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveGlobalState(t)
			t.Setenv("COMPOSE_ANSI", tt.env)
			unsetEnv(t, ComposeParallelLimit)
			unsetEnv(t, "NO_COLOR")
			// Keep the pre-run hook away from the developer's own project.
			unsetEnv(t, "COMPOSE_FILE")
			unsetEnv(t, "COMPOSE_ENV_FILES")
			t.Chdir(t.TempDir())

			out := streams.NewOut(io.Discard)
			cli := mocks.NewMockCli(gomock.NewController(t))
			cli.EXPECT().Out().Return(out).AnyTimes()
			cli.EXPECT().Err().Return(out).AnyTimes()

			// Run as a docker CLI plugin: compose is a child of the docker root.
			root := &cobra.Command{Use: "docker", TraverseChildren: true, SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(RootCommand(cli, &BackendOptions{}))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append([]string{PluginName}, tt.args...))
			assert.Error(t, root.Execute(), tt.wantErr)
		})
	}
}

func TestResolveMaxConcurrency(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     *string
		want    int
		wantErr string
	}{
		{name: "default without environment", want: -1},
		{name: "flag value without environment", args: []string{"--parallel", "2"}, want: 2},
		{name: "environment applies without flag", env: ptr("8"), want: 8},
		{name: "flag wins over environment", args: []string{"--parallel", "2"}, env: ptr("8"), want: 2},
		{name: "invalid environment", env: ptr("lots"), wantErr: `COMPOSE_PARALLEL_LIMIT must be an integer (found: "lots")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == nil {
				unsetEnv(t, ComposeParallelLimit)
			} else {
				t.Setenv(ComposeParallelLimit, *tt.env)
			}
			root := commandWithFlags(t, PluginName, tt.args...)
			sub := &cobra.Command{Use: "up"}
			root.AddCommand(sub)
			parallel, err := root.Flags().GetInt("parallel")
			assert.NilError(t, err)
			got, err := resolveMaxConcurrency(sub, parallel)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}

	t.Run("command outside of compose", func(t *testing.T) {
		_, err := resolveMaxConcurrency(&cobra.Command{Use: "other"}, 4)
		assert.Error(t, err, `error parsing command line, expected "compose"`)
	})
}

func TestNormalizeProjectOptions(t *testing.T) {
	cwd, err := filepath.Abs(".")
	assert.NilError(t, err)
	abs := filepath.Join(cwd, "abs.env")

	t.Run("deprecated workdir becomes project directory", func(t *testing.T) {
		opts := &ProjectOptions{WorkDir: "/work"}
		assert.NilError(t, normalizeProjectOptions(opts))
		assert.Equal(t, opts.ProjectDir, "/work")
	})
	t.Run("workdir conflicts with project directory", func(t *testing.T) {
		opts := &ProjectOptions{WorkDir: "/work", ProjectDir: "/project"}
		assert.Error(t, normalizeProjectOptions(opts), `cannot specify DEPRECATED "--workdir" and "--project-directory". Please use only "--project-directory" instead`)
		assert.Equal(t, opts.ProjectDir, "/project")
	})
	t.Run("env files are made absolute", func(t *testing.T) {
		opts := &ProjectOptions{EnvFiles: []string{"rel.env", abs}}
		assert.NilError(t, normalizeProjectOptions(opts))
		assert.DeepEqual(t, opts.EnvFiles, []string{filepath.Join(cwd, "rel.env"), abs})
	})
	t.Run("home directory is expanded", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		opts := &ProjectOptions{EnvFiles: []string{filepath.Join("~", "my.env")}}
		assert.NilError(t, normalizeProjectOptions(opts))
		assert.DeepEqual(t, opts.EnvFiles, []string{filepath.Join(home, "my.env")})
	})
}

func TestDefaultStringArrayVar(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "unset", want: []string{}},
		{name: "single value", value: "a.yaml", want: []string{"a.yaml"}},
		{name: "comma separated", value: "a.yaml,b.yaml", want: []string{"a.yaml", "b.yaml"}},
		{name: "empty items are dropped", value: ",a.yaml,,b.yaml,", want: []string{"a.yaml", "b.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COMPOSE_TEST_LIST", tt.value)
			assert.DeepEqual(t, defaultStringArrayVar("COMPOSE_TEST_LIST"), tt.want)
		})
	}
}
