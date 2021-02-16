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
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/progress"
)

type pullOptions struct {
	*projectOptions
	composeOptions
	quiet bool
}

func pullCommand(p *projectOptions) *cobra.Command {
	opts := pullOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pull [SERVICE...]",
		Short: "Pull service images",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd.Context(), opts, args)
		},
	}
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Pull without printing progress information")
	return cmd
}

func runPull(ctx context.Context, opts pullOptions, services []string) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	if opts.quiet {
		return c.ComposeService().Pull(ctx, project)
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Pull(ctx, project)
	})
	return err
}
