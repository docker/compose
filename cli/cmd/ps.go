package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/docker/api/client"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var PsCommand = cobra.Command{
	Use:   "ps",
	Short: "List containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		c, err := client.New(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot connect to backend")
		}

		containers, err := c.ContainerService().List(ctx)
		if err != nil {
			return errors.Wrap(err, "fetch containers")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tIMAGE\tCOMMAND\n")
		format := "%s\t%s\t%s\n"
		for _, c := range containers {
			fmt.Fprintf(w, format, c.ID, c.Image, c.Command)
		}
		return w.Flush()
	},
}
