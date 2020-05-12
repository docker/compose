package login

import (
	"github.com/spf13/cobra"
	"github.com/pkg/errors"
	"github.com/docker/api/client"
	apicontext "github.com/docker/api/context"
)

// Command returns the compose command with its child commands
func Command() *cobra.Command {
	command := &cobra.Command{
		Short: "Cloud login for docker contexts",
		Use:   "login",
	}
	command.AddCommand(
		azureLoginCommand(),
	)
	return command
}

func azureLoginCommand() *cobra.Command {
	azureLoginCmd := &cobra.Command{
		Use: "azure",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := apicontext.WithCurrentContext(cmd.Context(), "aci")
			c, err := client.New(ctx)
			if err != nil {
				return errors.Wrap(err, "cannot connect to backend")
			}
			return c.CloudService().Login(ctx, nil)
		},
	}

	return azureLoginCmd
}
