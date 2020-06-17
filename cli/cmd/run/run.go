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
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/docker/api/cli/options/run"
	"github.com/docker/api/client"
	"github.com/docker/api/progress"
)

// Command runs a container
func Command() *cobra.Command {
	var opts run.Opts
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd.Context(), args[0], opts)
		},
	}

	cmd.Flags().StringArrayVarP(&opts.Publish, "publish", "p", []string{}, "Publish a container's port(s). [HOST_PORT:]CONTAINER_PORT")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Assign a name to the container")
	cmd.Flags().StringArrayVarP(&opts.Labels, "label", "l", []string{}, "Set meta data on a container")
	cmd.Flags().StringArrayVarP(&opts.Volumes, "volume", "v", []string{}, "Volume. Ex: user:key@my_share:/absolute/path/to/target")

	return cmd
}

func runRun(ctx context.Context, image string, opts run.Opts) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	containerConfig, err := opts.ToContainerConfig(image)
	if err != nil {
		return err
	}

	eg, _ := errgroup.WithContext(ctx)
	w, err := progress.NewWriter(os.Stderr)
	if err != nil {
		return err
	}
	eg.Go(func() error {
		return w.Start(context.Background())
	})

	ctx = progress.WithContextWriter(ctx, w)

	eg.Go(func() error {
		defer w.Stop()
		return c.ContainerService().Run(ctx, containerConfig)
	})

	err = eg.Wait()
	fmt.Println(opts.Name)
	return err
}
