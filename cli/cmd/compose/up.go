/*
	Copyright (c) 2020 Docker Inc.

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package compose

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/compose"
)

func upCommand() *cobra.Command {
	opts := compose.ProjectOptions{}
	upCmd := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd.Context(), opts)
		},
	}
	upCmd.Flags().StringVarP(&opts.Name, "project-name", "p", "", "Project name")
	upCmd.Flags().StringVar(&opts.WorkDir, "workdir", ".", "Work dir")
	upCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	upCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")

	return upCmd
}

func runUp(ctx context.Context, opts compose.ProjectOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	composeService := c.ComposeService()
	if composeService == nil {
		return errors.New("compose not implemented in current context")
	}

	return composeService.Up(ctx, opts)
}
