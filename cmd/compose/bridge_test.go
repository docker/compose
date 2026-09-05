/*
   Copyright 2025 Docker Compose CLI authors

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
	"testing"

	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"
)

func TestBridgeCommandsArgsValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
	}{
		{
			name:    "create requires a PATH argument",
			cmd:     createTransformerCommand(nil),
			args:    []string{},
			wantErr: "requires 1 argument",
		},
		{
			name:    "create rejects extra arguments",
			cmd:     createTransformerCommand(nil),
			args:    []string{"dest", "extra"},
			wantErr: "requires 1 argument",
		},
		{
			name:    "convert rejects arguments",
			cmd:     convertCommand(&ProjectOptions{}, nil),
			args:    []string{"extra"},
			wantErr: "unknown command",
		},
		{
			name:    "list rejects arguments",
			cmd:     listTransformersCommand(nil),
			args:    []string{"extra"},
			wantErr: "unknown command",
		},
		{
			name:    "bridge rejects an unknown subcommand",
			cmd:     bridgeCommand(&ProjectOptions{}, nil),
			args:    []string{"zzz"},
			wantErr: "unknown docker command",
		},
		{
			name: "bridge with no subcommand shows help",
			cmd:  bridgeCommand(&ProjectOptions{}, nil),
			args: []string{},
		},
		{
			name:    "transformations rejects an unknown subcommand",
			cmd:     bridgeCommand(&ProjectOptions{}, nil),
			args:    []string{"transformations", "zzz"},
			wantErr: "unknown docker command",
		},
		{
			name: "transformations with no subcommand shows help",
			cmd:  transformersCommand(nil),
			args: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.cmd.SetArgs(test.args)
			test.cmd.SetOut(io.Discard)
			test.cmd.SetErr(io.Discard)
			err := test.cmd.Execute()
			if test.wantErr == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, test.wantErr)
			}
		})
	}
}
