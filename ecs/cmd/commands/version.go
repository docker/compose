package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version is the git tag that this was built from.
	Version = "unknown"
	// GitCommit is the commit that this was built from.
	GitCommit = "unknown"
)

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Docker ECS plugin %s (%s)\n", Version, GitCommit)
			return nil
		},
	}
}
