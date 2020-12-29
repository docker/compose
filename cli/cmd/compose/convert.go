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
	"fmt"

	"github.com/docker/compose-cli/api/compose"

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
)

func convertCommand() *cobra.Command {
	opts := composeOptions{}
	convertCmd := &cobra.Command{
		Use:   "convert",
		Short: "Converts the compose file to a cloud format (default: cloudformation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd.Context(), opts)
		},
	}
	convertCmd.Flags().StringVarP(&opts.ProjectName, "project-name", "p", "", "Project name")
	convertCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	convertCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	convertCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")
	convertCmd.Flags().StringVar(&opts.Format, "format", "yaml", "Format the output. Values: [yaml | json]")

	return convertCmd
}

func runConvert(ctx context.Context, opts composeOptions) error {
	var json []byte
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	options, err := opts.toProjectOptions()
	if err != nil {
		return err
	}

	project, err := cli.ProjectFromOptions(options)
	if err != nil {
		return err
	}

	json, err = c.ComposeService().Convert(ctx, project, compose.ConvertOptions{
		Format: opts.Format,
	})
	if err != nil {
		return err
	}

	fmt.Println(string(json))
	return nil
}
