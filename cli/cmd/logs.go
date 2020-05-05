package cmd

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/containers"
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
		Writer: os.Stdout,
	}

	return c.AciService().Logs(ctx, containerName, req)
}
