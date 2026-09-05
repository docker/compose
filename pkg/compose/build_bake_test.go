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
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/streams"
	"github.com/moby/buildkit/util/progress/progressui"
	"gotest.tools/v3/assert"
)

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
	t.Run("terminal stdout yields the real file", func(t *testing.T) {
		s := streams.NewOut(os.Stdout)
		out := makeConsole(s)
		if s.IsTerminal() {
			assert.Equal(t, out, os.Stdout)
			return
		}
		assert.Check(t, out != os.Stdout, "non-console stdout must not be passed to ConsoleFromFile")
	})

	t.Run("redirected file is not treated as a console", func(t *testing.T) {
		r, w, err := os.Pipe()
		assert.NilError(t, err)
		t.Cleanup(func() {
			_ = r.Close()
			_ = w.Close()
		})
		s := streams.NewOut(w)
		out := makeConsole(s)
		assert.Check(t, out != w, "pipe fd must not be handed to ConsoleFromFile")
	})

	t.Run("file-less stream keeps the console.File wrapper", func(t *testing.T) {
		s := streams.NewOut(&bytes.Buffer{})
		out := makeConsole(s)
		if s.IsTerminal() {
			_, ok := out.(*_console)
			assert.Check(t, ok, "expected a *_console, got %T", out)
			return
		}
		assert.Equal(t, out, io.Writer(s))
	})

	t.Run("plain writer is left untouched", func(t *testing.T) {
		buf := &bytes.Buffer{}
		assert.Equal(t, makeConsole(buf), io.Writer(buf))
	})
}

func TestNewBakeDisplayFallsBackWhenNotConsole(t *testing.T) {
	r, w, err := os.Pipe()
	assert.NilError(t, err)
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	_, err = newBakeDisplay(streams.NewOut(w), progressui.TtyMode)
	assert.NilError(t, err)
}
