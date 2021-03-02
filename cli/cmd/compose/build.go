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
	"github.com/docker/compose-cli/api/compose"
	"os"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/progress"
)

type buildOptions struct {
	*projectOptions
	composeOptions
	quiet bool
	pull  bool
}

func buildCommand(p *projectOptions) *cobra.Command {
	opts := buildOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "build [SERVICE...]",
		Short: "Build or rebuild services",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.quiet {
				devnull, err := os.Open(os.DevNull)
				if err != nil {
					return err
				}
				os.Stdout = devnull
			}
			return runBuild(cmd.Context(), opts, args)
		},
	}
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Don't print anything to STDOUT")
	cmd.Flags().BoolVar(&opts.pull, "pull", false, "Always attempt to pull a newer version of the image.")
	return cmd
}

func runBuild(ctx context.Context, opts buildOptions, services []string) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Build(ctx, project, compose.BuildOptions{
			Pull: opts.pull,
		})
	})
	return err
}
