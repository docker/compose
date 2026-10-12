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
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestLocalBuildPaths(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, "app")
	dockerfileDir := filepath.Join(root, "docker")
	assert.NilError(t, os.MkdirAll(contextDir, 0o755))
	assert.NilError(t, os.MkdirAll(dockerfileDir, 0o755))
	resolvedDir, err := filepath.EvalSymlinks(dockerfileDir)
	assert.NilError(t, err)
	secretFile := filepath.Join(root, "credentials")
	project := &types.Project{Secrets: types.Secrets{
		"file":   {File: secretFile},
		"env":    {Environment: "TOKEN"},
		"unused": {File: filepath.Join(root, "unused")},
	}}

	for _, dockerfile := range []string{filepath.Join("..", "docker", "Dockerfile"), filepath.Join(dockerfileDir, "Dockerfile")} {
		t.Run(dockerfile, func(t *testing.T) {
			config := types.BuildConfig{
				Context:            contextDir,
				Dockerfile:         dockerfile,
				AdditionalContexts: map[string]string{"remote": "https://example.com/context.git"},
				Secrets: []types.ServiceSecretConfig{
					{Source: "file", Target: "renamed"},
					{Source: "env"},
				},
			}
			assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir, filepath.Join(resolvedDir, "Dockerfile"), secretFile})
		})
	}

	for _, remote := range []string{"https://example.com/context.git", "git@github.com:docker/compose.git"} {
		t.Run(remote, func(t *testing.T) {
			config := types.BuildConfig{Context: remote, Dockerfile: "Dockerfile", Secrets: []types.ServiceSecretConfig{{Source: "file"}}}
			assert.DeepEqual(t, localBuildPaths(project, config), []string{secretFile})
		})
	}

	t.Run("remote context with local Dockerfile", func(t *testing.T) {
		config := types.BuildConfig{Context: "https://example.com/context.git", Dockerfile: filepath.Join(dockerfileDir, "Dockerfile")}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{config.Dockerfile})
	})

	t.Run("inline Dockerfile", func(t *testing.T) {
		config := types.BuildConfig{Context: contextDir, DockerfileInline: "FROM scratch"}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir})
	})

	t.Run("additional local context", func(t *testing.T) {
		config := types.BuildConfig{Context: contextDir, AdditionalContexts: map[string]string{"local": dockerfileDir}}
		assert.DeepEqual(t, localBuildPaths(project, config), []string{contextDir, dockerfileDir})
	})
}

func TestBakeEntitlementGrants(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config types.BuildConfig
		want   []string
	}{
		{
			name: "none",
		},
		{
			name:   "explicit host network",
			config: types.BuildConfig{Entitlements: []string{"network.host"}},
			want:   []string{"network.host"},
		},
		{
			name:   "host network mode",
			config: types.BuildConfig{Network: "host"},
			want:   []string{"network.host"},
		},
		{
			name:   "privileged",
			config: types.BuildConfig{Privileged: true},
			want:   []string{"security.insecure"},
		},
		{
			name:   "explicit insecure",
			config: types.BuildConfig{Entitlements: []string{"security.insecure"}},
			want:   []string{"security.insecure"},
		},
		{
			name:   "already requested",
			config: types.BuildConfig{Network: "host", Privileged: true, Entitlements: []string{"network.host", "security.insecure"}},
			want:   []string{"network.host", "security.insecure"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := slices.Clone(tc.config.Entitlements)
			entitlements := bakeEntitlements(tc.config)
			assert.DeepEqual(t, entitlements, tc.want)
			assert.DeepEqual(t, tc.config.Entitlements, original)
			args := bakeArgs(&bakeBuild{entitlements: append(entitlements, entitlements...)}, "metadata.json", api.BuildOptions{})
			want := []string{"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json"}
			for _, entitlement := range tc.want {
				want = append(want, "--allow", entitlement)
			}
			assert.DeepEqual(t, args, want)
		})
	}
}

func TestBakeReadGrants(t *testing.T) {
	bake := &bakeBuild{localPaths: []string{"/project", "/outside/Dockerfile", "/secrets/token", "/project"}}
	assert.DeepEqual(t, bakeArgs(bake, "metadata.json", api.BuildOptions{}), []string{
		"bake", "--file", "-", "--progress", "rawjson", "--metadata-file", "metadata.json",
		"--allow", "fs.read=/outside/Dockerfile", "--allow", "fs.read=/project", "--allow", "fs.read=/secrets/token",
	})
}

func TestPrepareBakeBuildGrants(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, "app")
	secretFile := filepath.Join(root, "token")
	dockerfile := filepath.Join(root, "docker", "Dockerfile")
	project := &types.Project{
		Name: "grants",
		Services: types.Services{"app": {Name: "app", WorkloadSpec: types.WorkloadSpec{Build: &types.BuildConfig{
			Context: contextDir, Dockerfile: dockerfile,
			Secrets:      []types.ServiceSecretConfig{{Source: "token"}},
			Entitlements: []string{"network.host"},
		}}}},
		Secrets: types.Secrets{"token": {File: secretFile}},
	}
	s := &composeService{proxyConfig: map[string]string{}, events: &ignore{}}
	bake := s.prepareBakeBuild(project, project.Services, api.BuildOptions{})
	assert.DeepEqual(t, bake.cfg.Targets["app"].Entitlements, []string{"network.host"})
	assert.DeepEqual(t, bake.cfg.Targets["app"].Secrets, []string{"id=token,type=file,src=" + secretFile})
	args := bakeArgs(bake, "metadata.json", api.BuildOptions{})
	for _, grant := range []string{"fs.read=" + contextDir, "fs.read=" + dockerfile, "fs.read=" + secretFile, "network.host"} {
		index := slices.Index(args, grant)
		assert.Assert(t, index > 0, "missing grant %s in %v", grant, args)
		assert.Equal(t, args[index-1], "--allow")
	}
	assert.Assert(t, !slices.Contains(args, "fs.read="+filepath.Dir(dockerfile)), "must not grant the Dockerfile's parent directory")
}

func TestBakeTargetNames(t *testing.T) {
	project := &types.Project{
		Services: types.Services{
			"web":   {},
			"a.b":   {},
			"a_b":   {},
			"a.b.c": {},
			"a_b.c": {},
		},
	}

	names := bakeTargetNames(project)

	// dots are replaced, and services whose names only differ by `.` vs `_`
	// still get distinct bake targets, allocated in sorted service order
	assert.DeepEqual(t, names, map[string]string{
		"web":   "web",
		"a.b":   "a_b",
		"a_b":   "a_b_",
		"a.b.c": "a_b_c",
		"a_b.c": "a_b_c_",
	})
}

func TestToBakeAttest(t *testing.T) {
	tests := []struct {
		name     string
		config   types.BuildConfig
		expected []string
	}{
		{
			name:     "empty — no attest entries",
			config:   types.BuildConfig{},
			expected: nil,
		},
		{
			name:     "provenance true",
			config:   types.BuildConfig{Provenance: "true"},
			expected: []string{"type=provenance"},
		},
		{
			name:     "provenance false — must disable, not omit",
			config:   types.BuildConfig{Provenance: "false"},
			expected: []string{"type=provenance,disabled=true"},
		},
		{
			name:     "provenance mode=max",
			config:   types.BuildConfig{Provenance: "mode=max"},
			expected: []string{"type=provenance,mode=max"},
		},
		{
			name:     "sbom true",
			config:   types.BuildConfig{SBOM: "true"},
			expected: []string{"type=sbom"},
		},
		{
			name:     "sbom false — must disable, not omit",
			config:   types.BuildConfig{SBOM: "false"},
			expected: []string{"type=sbom,disabled=true"},
		},
		{
			name:     "provenance false + sbom false",
			config:   types.BuildConfig{Provenance: "false", SBOM: "false"},
			expected: []string{"type=provenance,disabled=true", "type=sbom,disabled=true"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.DeepEqual(t, toBakeAttest(tc.config), tc.expected)
		})
	}
}

// makeConsole must hand the genuine *os.File over when the stream wraps one:
// on Windows, containerd/console rejects anything but the exact
// os.Stdin/Stdout/Stderr values, so a wrapper would disable the TTY progress
// rendering entirely (#14086).
func TestMakeConsole(t *testing.T) {
	t.Run("stream wrapping a real file yields the file itself", func(t *testing.T) {
		out := makeConsole(streams.NewOut(os.Stdout))
		assert.Equal(t, out, os.Stdout)
	})

	t.Run("file-less stream keeps the console.File wrapper", func(t *testing.T) {
		out := makeConsole(streams.NewOut(&bytes.Buffer{}))
		_, ok := out.(*_console)
		assert.Check(t, ok, "expected a *_console, got %T", out)
	})

	t.Run("plain writer is left untouched", func(t *testing.T) {
		buf := &bytes.Buffer{}
		assert.Equal(t, makeConsole(buf), io.Writer(buf))
	})
}
