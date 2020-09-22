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

package cmd

import (
	"context"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/containers"
)

type logsOpts struct {
	Follow bool
	Tail   string
}

// LogsCommand fetches and shows logs of a container
func LogsCommand() *cobra.Command {
	var opts logsOpts
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Fetch the logs of a container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd.Context(), args[0], opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "Follow log outut")
	cmd.Flags().StringVar(&opts.Tail, "tail", "all", "Number of lines to show from the end of the logs")

	return cmd
}

func runLogs(ctx context.Context, containerName string, opts logsOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	req := containers.LogsRequest{
		Follow: opts.Follow,
		Tail:   opts.Tail,
	}

	var con io.Writer = os.Stdout
	if c, err := console.ConsoleFromFile(os.Stdout); err == nil {
		size, err := c.Size()
		if err != nil {
			return err
		}
		req.Width = int(size.Width)
		con = c
	}

	req.Writer = con

	return c.ContainerService().Logs(ctx, containerName, req)
}
