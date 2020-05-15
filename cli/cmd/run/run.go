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

package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
)

// Command runs a container
func Command() *cobra.Command {
	var opts runOpts
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), args[0], opts)
		},
	}

	cmd.Flags().StringArrayVarP(&opts.publish, "publish", "p", []string{}, "Publish a container's port(s). [HOST_PORT:]CONTAINER_PORT")
	cmd.Flags().StringVar(&opts.name, "name", getRandomName(), "Assign a name to the container")

	return cmd
}

func runRun(ctx context.Context, image string, opts runOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toContainerConfig(image)
	if err != nil {
		return err
	}

	if err = c.ContainerService().Run(ctx, project); err != nil {
		return err
	}
	fmt.Println(opts.name)
	return nil

}

func getRandomName() string {
	// Azure supports hyphen but not underscore in names
	return strings.Replace(namesgenerator.GetRandomName(0), "_", "-", -1)
}
