package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
)

type psOpts struct {
	quiet bool
}

// PsCommand lists containers
func PsCommand() *cobra.Command {
	var opts psOpts
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPs(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Only display IDs")

	return cmd
}

func runPs(ctx context.Context, opts psOpts) error {
	c, err := client.New(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot connect to backend")
	}

	containers, err := c.ContainerService().List(ctx)
	if err != nil {
		return errors.Wrap(err, "fetch containers")
	}

	if opts.quiet {
		for _, c := range containers {
			fmt.Println(c.ID)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tIMAGE\tSTATUS\tCOMMAND\n")
	format := "%s\t%s\t%s\t%s\n"
	for _, c := range containers {
		fmt.Fprintf(w, format, c.ID, c.Image, c.Status, c.Command)
	}

	return w.Flush()
}
