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
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/pkg/api"
)

func TestLocalBuildPathsIncludesDockerfileOutsideContext(t *testing.T) {
	ctxDir := t.TempDir()
	outside := t.TempDir()
	df := filepath.Join(outside, "Dockerfile")
	assert.NilError(t, os.WriteFile(df, []byte("FROM scratch\n"), 0o644))

	resolvedDir, err := filepath.EvalSymlinks(outside)
	assert.NilError(t, err)

	got := localBuildPaths(types.BuildConfig{
		Context:    ctxDir,
		Dockerfile: df,
		AdditionalContexts: map[string]string{
			"extra": "https://github.com/docker/compose.git",
			"same":  ctxDir,
		},
	})
	wantFile := filepath.Join(resolvedDir, "Dockerfile")
	assert.Assert(t, containsPath(got, ctxDir))
	assert.Assert(t, containsPath(got, resolvedDir))
	assert.Assert(t, !containsPath(got, wantFile))
	assert.Equal(t, 2, len(got))
	for _, p := range got {
		assert.Assert(t, !strings.Contains(p, "://"), "remote context should not be granted: %s", p)
	}

	inside := filepath.Join(ctxDir, "Dockerfile")
	assert.NilError(t, os.WriteFile(inside, []byte("FROM scratch\n"), 0o644))
	insideGot := localBuildPaths(types.BuildConfig{
		Context:    ctxDir,
		Dockerfile: "Dockerfile",
	})
	assert.DeepEqual(t, insideGot, []string{ctxDir})

	gitish := filepath.Join(outside, "github.com", "org", "repo")
	assert.NilError(t, os.MkdirAll(gitish, 0o755))
	gitDF := filepath.Join(gitish, "Dockerfile")
	assert.NilError(t, os.WriteFile(gitDF, []byte("FROM scratch\n"), 0o644))
	gitishResolved, err := filepath.EvalSymlinks(gitish)
	assert.NilError(t, err)
	gitGot := localBuildPaths(types.BuildConfig{
		Context:    ctxDir,
		Dockerfile: gitDF,
	})
	assert.Assert(t, containsPath(gitGot, gitishResolved))

	remote := localBuildPaths(types.BuildConfig{
		Context:    "https://github.com/docker/compose.git",
		Dockerfile: "Dockerfile",
	})
	assert.Equal(t, len(remote), 0)

	args := bakeArgs(&bakeBuild{localPaths: got}, "meta.json", api.BuildOptions{})
	assert.Assert(t, containsPath(allowPaths(args), resolvedDir))
	assert.Assert(t, !containsPath(allowPaths(args), wantFile))
}

func allowPaths(args []string) []string {
	var paths []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--allow" && strings.HasPrefix(args[i+1], "fs.read=") {
			paths = append(paths, strings.TrimPrefix(args[i+1], "fs.read="))
		}
	}
	return paths
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
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
