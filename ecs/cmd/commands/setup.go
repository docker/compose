package commands

import (
	"github.com/docker/cli/cli-plugins/plugin"
	contextStore "github.com/docker/ecs-plugin/pkg/docker"
	"github.com/spf13/cobra"
)

func SetupCommand() *cobra.Command {
	var opts contextStore.AwsContext
	var name string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			//Override the root command PersistentPreRun
			//We just need to initialize the top parent command
			return plugin.PersistentPreRunE(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return contextStore.NewContext(name, &opts)
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "aws", "Context Name")
	cmd.Flags().StringVarP(&opts.Profile, "profile", "p", "", "AWS Profile")
	cmd.Flags().StringVarP(&opts.Cluster, "cluster", "c", "", "ECS cluster")
	cmd.Flags().StringVarP(&opts.Region, "region", "r", "", "AWS region")

	cmd.MarkFlagRequired("profile")
	cmd.MarkFlagRequired("cluster")
	cmd.MarkFlagRequired("region")
	return cmd
}
