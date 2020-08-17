package commands

import (
	"fmt"

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"
)

// NewRootCmd returns the base root command.
func NewRootCmd(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Short:       "Docker ECS",
		Long:        `run multi-container applications on Amazon ECS.`,
		Use:         "ecs",
		Annotations: map[string]string{"experimentalCLI": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("%q is not a docker ecs command\nSee 'docker ecs --help'", args[0])
			}
			cmd.Help()
			return nil
		},
	}
	cmd.AddCommand(
		VersionCommand(),
		ComposeCommand(dockerCli),
		SecretCommand(dockerCli),
		SetupCommand(),
	)
	return cmd
}
