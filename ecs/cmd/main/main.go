package main

import (
	"fmt"

	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	commands "github.com/docker/ecs-plugin/cmd/commands"
	"github.com/docker/ecs-plugin/pkg/docker"
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
	var opts *docker.AwsContext

	cmd := &cobra.Command{
		Short:       "Docker ECS",
		Long:        `run multi-container applications on Amazon ECS.`,
		Use:         name,
		Annotations: map[string]string{"experimentalCLI": "true"},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := plugin.PersistentPreRunE(cmd, args)
			if err != nil {
				return err
			}
			contextName := dockerCli.CurrentContext()
			opts, err = docker.CheckAwsContextExists(contextName)
			return err
		},
	}
	cmd.AddCommand(
		VersionCommand(),
		commands.ComposeCommand(opts),
		commands.SecretCommand(opts),
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
