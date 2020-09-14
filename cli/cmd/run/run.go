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

package run

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/containers"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/cli/options/run"
	"github.com/docker/compose-cli/progress"
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
	cmd.Flags().StringArrayVarP(&opts.Volumes, "volume", "v", []string{}, "Volume. Ex: storageaccount/my_share[:/absolute/path/to/target][:ro]")
	cmd.Flags().BoolVarP(&opts.Detach, "detach", "d", false, "Run container in background and print container ID")
	cmd.Flags().Float64Var(&opts.Cpus, "cpus", 1., "Number of CPUs")
	cmd.Flags().VarP(&opts.Memory, "memory", "m", "Memory limit")
	cmd.Flags().StringArrayVarP(&opts.Environment, "env", "e", []string{}, "Set environment variables")
	cmd.Flags().StringVarP(&opts.RestartPolicyCondition, "restart", "", containers.RestartPolicyNone, "Restart policy to apply when a container exits")

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

	err = progress.Run(ctx, func(ctx context.Context) error {
		return c.ContainerService().Run(ctx, containerConfig)
	})
	if err != nil {
		return err
	}

	if !opts.Detach {
		var con io.Writer = os.Stdout
		req := containers.LogsRequest{
			Follow: true,
		}
		if c, err := console.ConsoleFromFile(os.Stdout); err == nil {
			size, err := c.Size()
			if err != nil {
				return err
			}
			req.Width = int(size.Width)
			con = c
		}

		req.Writer = con

		return c.ContainerService().Logs(ctx, opts.Name, req)
	}

	fmt.Println(opts.Name)

	return nil
}
