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

package compatibility

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_convert(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "compose only",
			args: []string{"up"},
			want: []string{"compose", "up"},
		},
		{
			name: "with context",
			args: []string{"--context", "foo", "-f", "compose.yaml", "up"},
			want: []string{"--context", "foo", "compose", "-f", "compose.yaml", "up"},
		},
		{
			name: "with context arg",
			args: []string{"--context=foo", "-f", "compose.yaml", "up"},
			want: []string{"--context", "foo", "compose", "-f", "compose.yaml", "up"},
		},
		{
			name: "with host",
			args: []string{"--host", "tcp://1.2.3.4", "up"},
			want: []string{"--host", "tcp://1.2.3.4", "compose", "up"},
		},
		{
			name: "compose --verbose",
			args: []string{"--verbose"},
			want: []string{"--debug", "compose"},
		},
		{
			name: "compose --version",
			args: []string{"--version"},
			want: []string{"compose", "version"},
		},
		{
			name: "compose -v",
			args: []string{"-v"},
			want: []string{"compose", "version"},
		},
		{
			name: "help",
			args: []string{"-h"},
			want: []string{"compose", "--help"},
		},
		{
			name: "issues/1962",
			args: []string{"psql", "-h", "postgres"},
			want: []string{"compose", "psql", "-h", "postgres"}, // -h should not be converted to --help
		},
		{
			name: "issues/8648",
			args: []string{"exec", "mongo", "mongo", "--host", "mongo"},
			want: []string{"compose", "exec", "mongo", "mongo", "--host", "mongo"}, // --host is passed to exec
		},
		{
			name: "issues/12",
			args: []string{"--log-level", "INFO", "up"},
			want: []string{"--log-level", "INFO", "compose", "up"},
		},
		{
			name: "empty string argument",
			args: []string{"--project-directory", "", "ps"},
			want: []string{"compose", "--project-directory", "", "ps"},
		},
		{
			name: "compose as project name",
			args: []string{"--project-name", "compose", "down", "--remove-orphans"},
			want: []string{"compose", "--project-name", "compose", "down", "--remove-orphans"},
		},
		{
			name: "completion command",
			args: []string{"__complete", "up"},
			want: []string{"__complete", "compose", "up"},
		},
		{
			name:    "string flag without argument",
			args:    []string{"--log-level"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				if os.Getenv("BE_CRASHER") == "1" {
					Convert(tt.args)
					return
				}
				cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
				cmd.Env = append(os.Environ(), "BE_CRASHER=1")
				err := cmd.Run()
				var e *exec.ExitError
				if errors.As(err, &e) && !e.Success() {
					return
				}
				t.Fatalf("process ran with err %v, want exit status 1", err)
			} else {
				got := Convert(tt.args)
				assert.DeepEqual(t, tt.want, got)
			}
		})
	}
}
