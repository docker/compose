package commands

import (
	"fmt"

	"github.com/docker/ecs-plugin/internal"

	"github.com/spf13/cobra"
)

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Docker ECS plugin %s (%s)\n", internal.Version, internal.GitCommit)
			return nil
		},
	}
}
