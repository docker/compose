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
