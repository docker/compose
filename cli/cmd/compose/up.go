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
	"errors"

	"github.com/compose-spec/compose-go/cli"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/progress"
)

func upCommand() *cobra.Command {
	opts := cli.ProjectOptions{}
	upCmd := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd.Context(), &opts)
		},
	}
	upCmd.Flags().StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	upCmd.Flags().StringVar(&opts.WorkingDir, "workdir", ".", "Work dir")
	upCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	upCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")
	upCmd.Flags().BoolP("detach", "d", true, " Detached mode: Run containers in the background")

	return upCmd
}

func runUp(ctx context.Context, opts *cli.ProjectOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	composeService := c.ComposeService()
	if composeService == nil {
		return errors.New("compose not implemented in current context")
	}

	return progress.Run(ctx, func(ctx context.Context) error {
		return composeService.Up(ctx, opts)
	})
}
