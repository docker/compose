package login

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
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
			ctx := cmd.Context()
			cs, err := client.GetCloudService(ctx, "aci")
			if err != nil {
				return errors.Wrap(err, "cannot connect to backend")
			}
			return cs.Login(ctx, nil)
		},
	}

	return azureLoginCmd
}
