package context

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
)

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available contexts",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context())
		},
	}
	return cmd
}

func runList(ctx context.Context) error {
	s := store.ContextStore(ctx)
	contexts, err := s.List()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tTYPE")
	format := "%s\t%s\t%s\n"

	for _, c := range contexts {
		fmt.Fprintf(w, format, c.Name, c.Metadata.Description, c.Metadata.Type)
	}

	return w.Flush()
}
