package login

import (
	"github.com/spf13/cobra"
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
			return nil
		},
	}

	return azureLoginCmd
}
