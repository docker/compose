/*
   Copyright 2026 Docker Compose CLI authors

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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/cmd/display"
)

// display.Mode resolves "auto" against stderr — the stream Compose's own
// progress renders to — while bake renders on stdout. toAPIBuildOptions must
// therefore hand the unresolved "auto" back to bake unless the user explicitly
// asked for tty, so buildkit probes the actual output stream and degrades to
// plain when stdout is redirected (#14182).
func TestToAPIBuildOptionsProgress(t *testing.T) {
	tests := []struct {
		name     string
		mode     string // display.Mode as resolved by selectEventProcessor
		progress string // raw --progress flag value
		want     string
	}{
		{name: "auto resolved to tty is handed back as auto", mode: display.ModeTTY, progress: "", want: display.ModeAuto},
		{name: "explicit tty is passed through", mode: display.ModeTTY, progress: display.ModeTTY, want: display.ModeTTY},
		{name: "plain is passed through", mode: display.ModePlain, progress: "", want: display.ModePlain},
		{name: "quiet is passed through", mode: display.ModeQuiet, progress: "", want: display.ModeQuiet},
		{name: "json maps to rawjson", mode: display.ModeJSON, progress: display.ModeJSON, want: "rawjson"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := display.Mode
			t.Cleanup(func() { display.Mode = mode })
			display.Mode = tt.mode

			opts := buildOptions{ProjectOptions: &ProjectOptions{Progress: tt.progress}}
			bo, err := opts.toAPIBuildOptions(nil)
			assert.NilError(t, err)
			assert.Equal(t, bo.Progress, tt.want)
		})
	}
}
