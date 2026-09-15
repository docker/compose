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
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	dockercli "github.com/docker/cli/cli"
	"github.com/spf13/cobra"
	"gotest.tools/v3/assert"

	"github.com/docker/compose/v5/cmd/prompt"
)

func TestFilterServices(t *testing.T) {
	p := &types.Project{
		Services: types.Services{
			"foo": {
				Name:  "foo",
				Links: []string{"bar"},
			},
			"bar": {
				Name: "bar",
				DependsOn: map[string]types.ServiceDependency{
					"zot": {},
				},
			},
			"zot": {
				Name: "zot",
			},
			"qix": {
				Name: "qix",
			},
		},
	}
	p, err := p.WithSelectedServices([]string{"bar"})
	assert.NilError(t, err)

	assert.Equal(t, len(p.Services), 2)
	_, err = p.GetService("bar")
	assert.NilError(t, err)
	_, err = p.GetService("zot")
	assert.NilError(t, err)
}

// Ctrl+C at an interactive prompt surfaces as prompt.ErrInterrupt (raw mode
// swallows the SIGINT): the command must exit with the same 130 status a real
// SIGINT produces, not a generic failure.
func TestAdaptCmdMapsPromptInterruptTo130(t *testing.T) {
	run := AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
		return fmt.Errorf("prompting: %w", prompt.ErrInterrupt)
	})
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	err := run(cmd, nil)
	var status dockercli.StatusError
	assert.Assert(t, errors.As(err, &status))
	assert.Equal(t, status.StatusCode, 130)
}
