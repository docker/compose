package cmd

import (
	"context"
	"os"

	"github.com/docker/api/client"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type logsOpts struct {
	Follow bool
}

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

	return cmd
}

func runLogs(ctx context.Context, name string, opts logsOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	return c.ContainerService().Logs(ctx, name, os.Stdout, opts.Follow)
}
