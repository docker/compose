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
	"github.com/docker/compose-cli/api/progress"
)

type pushOptions struct {
	composeOptions
}

func pushCommand() *cobra.Command {
	opts := pushOptions{}
	pushCmd := &cobra.Command{
		Use:   "push [SERVICE...]",
		Short: "Push service images",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd.Context(), opts, args)
		},
	}

	pushCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	pushCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return pushCmd
}

func runPush(ctx context.Context, opts pushOptions, services []string) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
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
		return "", c.ComposeService().Push(ctx, project)
	})
	return err
}
