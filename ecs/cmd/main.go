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
	var opts commands.ClusterOptions

	cmd := &cobra.Command{
		Short:       "Docker ECS",
		Long:        `run multi-container applications on Amazon ECS.`,
		Use:         name,
		Annotations: map[string]string{"experimentalCLI": "true"},
	}
	cmd.AddCommand(
		VersionCommand(),
		commands.ComposeCommand(&opts),
		commands.SecretCommand(&opts),
	)
	cmd.Flags().StringVarP(&opts.Profile, "profile", "p", "default", "AWS Profile")
	cmd.Flags().StringVarP(&opts.Cluster, "cluster", "c", "default", "ECS cluster")
	cmd.Flags().StringVarP(&opts.Region, "region", "r", "", "AWS region")

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
