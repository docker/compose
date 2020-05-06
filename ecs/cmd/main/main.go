package main

import (
	"fmt"

	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	commands "github.com/docker/ecs-plugin/cmd/commands"
	"github.com/spf13/cobra"
)

const version = "0.0.1"

func main() {
	plugin.Run(func(dockerCli command.Cli) *cobra.Command {
		cmd := NewRootCmd("ecs", dockerCli)
		return cmd
	}, manager.Metadata{
		SchemaVersion: "0.1.0",
		Vendor:        "Docker Inc.",
		Version:       version,
		Experimental:  true,
	})
}

// NewRootCmd returns the base root command.
func NewRootCmd(name string, dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Short:       "Docker ECS",
		Long:        `run multi-container applications on Amazon ECS.`,
		Use:         name,
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
		commands.ComposeCommand(dockerCli),
		commands.SecretCommand(dockerCli),
		commands.SetupCommand(),
	)
	return cmd
}

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Docker ECS plugin %s\n", version)
			return nil
		},
	}
}
