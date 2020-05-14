package cmd

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/multierror"
)

type rmOpts struct {
	force bool
}

// RmCommand deletes containers
func RmCommand() *cobra.Command {
	var opts rmOpts
	cmd := &cobra.Command{
		Use:     "rm",
		Aliases: []string{"delete"},
		Short:   "Remove containers",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return errors.Wrap(err, "cannot connect to backend")
			}

			var errs *multierror.Error
			for _, id := range args {
				err := c.ContainerService().Delete(cmd.Context(), id, opts.force)
				if err != nil {
					errs = multierror.Append(errs, err)
					continue
				}
				fmt.Println(id)
			}

			return errs.ErrorOrNil()
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force removal")

	return cmd
}
