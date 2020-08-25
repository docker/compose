/*
   Copyright 2020 Docker, Inc.

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

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/client"
)

func convertCommand() *cobra.Command {
	opts := composeOptions{}
	convertCmd := &cobra.Command{
		Use:   "convert",
		Short: "Converts the compose file to a cloud format (default: cloudformation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := opts.toProjectOptions()
			if err != nil {
				return err
			}
			return runConvert(cmd.Context(), options)
		},
	}
	convertCmd.Flags().StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	convertCmd.Flags().StringVar(&opts.WorkingDir, "workdir", "", "Work dir")
	convertCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return convertCmd
}

func runConvert(ctx context.Context, opts *cli.ProjectOptions) error {
	var json []byte
	c, err := client.New(ctx)
	if err != nil {
		return err
	}
	json, err = c.ComposeService().Convert(ctx, opts)
	if err != nil {
		return err
	}

	fmt.Println(string(json))
	return nil
}
