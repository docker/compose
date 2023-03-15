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

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

// alphaCommand groups all experimental subcommands
func alphaCommand(p *ProjectOptions, backend api.Service) *cobra.Command {
	cmd := &cobra.Command{
		Short:  "Experimental commands",
		Use:    "alpha [COMMAND]",
		Hidden: true,
		Annotations: map[string]string{
			"experimentalCLI": "true",
		},
	}
	cmd.AddCommand(
		watchCommand(p, backend),
		dryRunRedirectCommand(p),
		vizCommand(p),
	)
	return cmd
}

// Temporary alpha command as the dry-run will be implemented with a flag
func dryRunRedirectCommand(p *ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dry-run -- [COMMAND...]",
		Short: "EXPERIMENTAL - Dry run command allow you to test a command without applying changes",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			rootCmd := cmd.Root()
			rootCmd.SetArgs(append([]string{"compose", "--dry-run"}, args...))
			return rootCmd.Execute()
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	return cmd
}
