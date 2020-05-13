package cmd

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
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
			var errs []string
			c, err := client.New(cmd.Context())
			if err != nil {
				return errors.Wrap(err, "cannot connect to backend")
			}

			for _, id := range args {
				err := c.ContainerService().Delete(cmd.Context(), id, opts.force)
				if err != nil {
					errs = append(errs, err.Error()+" "+id)
					continue
				}
				fmt.Println(id)
			}

			if len(errs) > 0 {
				return errors.New(strings.Join(errs, "\n"))
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force removal")

	return cmd
}
