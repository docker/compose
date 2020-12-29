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
	"os"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/formatter"
)

func logsCommand() *cobra.Command {
	opts := composeOptions{}
	logsCmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "View output from containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd.Context(), opts, args)
		},
	}
	logsCmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name")
	logsCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	logsCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return logsCmd
}

func runLogs(ctx context.Context, opts composeOptions, services []string) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	consumer := formatter.NewLogConsumer(ctx, os.Stdout)
	return c.ComposeService().Logs(ctx, projectName, consumer, compose.LogOptions{
		Services: services,
	})
}
