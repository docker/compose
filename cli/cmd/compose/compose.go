package compose

import (
	"github.com/spf13/cobra"
)

// Command returns the compose command with its child commands
func Command() *cobra.Command {
	command := &cobra.Command{
		Short: "Docker Compose",
		Use:   "compose",
	}

	command.AddCommand(
		upCommand(),
		downCommand(),
	)

	return command
}
