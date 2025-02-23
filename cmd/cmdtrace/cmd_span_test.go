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

package cmdtrace

import (
	"reflect"
	"testing"

	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

func TestGetFlags(t *testing.T) {
	// Initialize flagSet with flags
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	var (
		detach  string
		timeout string
	)
	fs.StringVar(&detach, "detach", "d", "")
	fs.StringVar(&timeout, "timeout", "t", "")
	_ = fs.Set("detach", "detach")
	_ = fs.Set("timeout", "timeout")

	tests := []struct {
		name     string
		input    *flag.FlagSet
		expected []string
	}{
		{
			name:     "NoFlags",
			input:    flag.NewFlagSet("NoFlags", flag.ContinueOnError),
			expected: nil,
		},
		{
			name:     "Flags",
			input:    fs,
			expected: []string{"detach", "timeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getFlags(test.input)
			if !reflect.DeepEqual(result, test.expected) {
				t.Errorf("Expected %v, but got %v", test.expected, result)
			}
		})
	}
}

func TestCommandName(t *testing.T) {
	tests := []struct {
		name     string
		setupCmd func() *cobra.Command
		want     []string
	}{
		{
			name: "docker compose alpha watch -> [watch, alpha]",
			setupCmd: func() *cobra.Command {
				dockerCmd := &cobra.Command{Use: "docker"}
				composeCmd := &cobra.Command{Use: commands.PluginName}
				alphaCmd := &cobra.Command{Use: "alpha"}
				watchCmd := &cobra.Command{Use: "watch"}

				dockerCmd.AddCommand(composeCmd)
				composeCmd.AddCommand(alphaCmd)
				alphaCmd.AddCommand(watchCmd)

				return watchCmd
			},
			want: []string{"watch", "alpha"},
		},
		{
			name: "docker-compose up -> [up]",
			setupCmd: func() *cobra.Command {
				dockerComposeCmd := &cobra.Command{Use: commands.PluginName}
				upCmd := &cobra.Command{Use: "up"}

				dockerComposeCmd.AddCommand(upCmd)

				return upCmd
			},
			want: []string{"up"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			got := commandName(cmd)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("commandName() = %v, want %v", got, tt.want)
			}
		})
	}
}
