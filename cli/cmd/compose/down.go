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

	"github.com/compose-spec/compose-go/types"

	"github.com/docker/compose-cli/api/compose"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/progress"
)

func downCommand() *cobra.Command {
	opts := composeOptions{}
	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Stop and remove containers, networks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(cmd.Context(), opts)
		},
	}
	downCmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name")
	downCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	downCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return downCmd
}

func runDown(ctx context.Context, opts composeOptions) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		name := opts.ProjectName
		var project *types.Project
		if opts.ProjectName == "" {
			p, err := opts.toProject()
			if err != nil {
				return "", err
			}
			project = p
			name = p.Name
		}

		return name, c.ComposeService().Down(ctx, name, compose.DownOptions{
			RemoveOrphans: false,
			Project:       project,
		})
	})
	return err
}
