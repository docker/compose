package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

const Version = "0.0.1"

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Docker ECS plugin %s\n", Version)
			return nil
		},
	}
}
