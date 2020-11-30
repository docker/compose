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

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/progress"
)

type buildOptions struct {
	composeOptions
}

func buildCommand() *cobra.Command {
	opts := buildOptions{}
	buildCmd := &cobra.Command{
		Use: "build [SERVICE...]",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd.Context(), opts, args)
		},
	}
	return buildCmd
}

func runBuild(ctx context.Context, opts buildOptions, services []string) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		options, err := opts.toProjectOptions()
		if err != nil {
			return "", err
		}
		project, err := cli.ProjectFromOptions(options)
		if err != nil {
			return "", err
		}

		err = filter(project, services)
		if err != nil {
			return "", err
		}
		return "", c.ComposeService().Build(ctx, project)
	})
	return err
}
