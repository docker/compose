package context

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/docker/api/context/store"
	"github.com/docker/api/multierror"
)

func removeCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "rm CONTEXT [CONTEXT...]",
		Short:   "Remove one or more contexts",
		Aliases: []string{"remove"},
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), args)
		},
	}
}

func runRemove(ctx context.Context, args []string) error {
	s := store.ContextStore(ctx)
	var errs *multierror.Error
	for _, n := range args {
		if err := s.Remove(n); err != nil {
			errs = multierror.Append(errs, err)
		} else {
			fmt.Println(n)
		}
	}
	return errs.ErrorOrNil()
}
