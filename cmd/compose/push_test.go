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
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
)

func TestPushIgnorePushFailuresIsNotDiscoverable(t *testing.T) {
	cmd := pushCommand(&ProjectOptions{}, nil, &BackendOptions{})
	assert.Assert(t, !strings.Contains(cmd.UsageString(), "ignore-push-failures"))
}

// The real push command is run with RunE stubbed out, so only flag parsing
// and validation are exercised.
func TestPushIgnoreFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantWarning bool
		wantErr     string
	}{
		{name: "ignore-push-failures is deprecated but usable", args: []string{"--ignore-push-failures"}, wantWarning: true},
		{name: "ignore-missing alone", args: []string{"--ignore-missing"}},
		{name: "both flags are mutually exclusive", args: []string{"--ignore-push-failures", "--ignore-missing"}, wantWarning: true, wantErr: "none of the others can be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := pushCommand(&ProjectOptions{}, nil, &BackendOptions{})
			cmd.RunE = func(*cobra.Command, []string) error { return nil }
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)

			err := cmd.ExecuteContext(t.Context())
			if tt.wantErr == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
			assert.Equal(t, strings.Contains(out.String(), "--ignore-push-failures has been deprecated"), tt.wantWarning, out.String())
		})
	}
}
