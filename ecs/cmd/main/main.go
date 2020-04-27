package main

import (
	"context"
	"fmt"

	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/docker/ecs-plugin/pkg/amazon"
	"github.com/docker/ecs-plugin/pkg/compose"
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

type clusterOptions struct {
	profile string
	region  string
	cluster string
}

// NewRootCmd returns the base root command.
func NewRootCmd(name string, dockerCli command.Cli) *cobra.Command {
	var opts clusterOptions

	cmd := &cobra.Command{
		Short:       "Docker ECS",
		Long:        `run multi-container applications on Amazon ECS.`,
		Use:         name,
		Annotations: map[string]string{"experimentalCLI": "true"},
	}
	cmd.AddCommand(
		VersionCommand(),
		ComposeCommand(&opts),
	)
	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "default", "AWS Profile")
	cmd.Flags().StringVarP(&opts.cluster, "cluster", "c", "default", "ECS cluster")
	cmd.Flags().StringVarP(&opts.region, "region", "r", "", "AWS region")

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

func ComposeCommand(clusteropts *clusterOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "compose",
	}
	opts := &compose.ProjectOptions{}
	opts.AddFlags(cmd.Flags())

	cmd.AddCommand(
		ConvertCommand(clusteropts, opts),
		UpCommand(clusteropts, opts),
		DownCommand(clusteropts, opts),
	)
	return cmd
}

type upOptions struct {
	loadBalancerArn string
}

func (o upOptions) LoadBalancerArn() *string {
	if o.loadBalancerArn == "" {
		return nil
	}
	return &o.loadBalancerArn
}

func ConvertCommand(clusteropts *clusterOptions, projectOpts *compose.ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "convert",
		RunE: compose.WithProject(projectOpts, func(project *compose.Project, args []string) error {
			client, err := amazon.NewClient(clusteropts.profile, clusteropts.cluster, clusteropts.region)
			if err != nil {
				return err
			}
			template, err := client.Convert(context.Background(), project)
			if err != nil {
				return err
			}

			j, err := template.JSON()
			if err != nil {
				fmt.Printf("Failed to generate JSON: %s\n", err)
			} else {
				fmt.Printf("%s\n", string(j))
			}
			return nil
		}),
	}
	return cmd
}

func UpCommand(clusteropts *clusterOptions, projectOpts *compose.ProjectOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "up",
		RunE: compose.WithProject(projectOpts, func(project *compose.Project, args []string) error {
			client, err := amazon.NewClient(clusteropts.profile, clusteropts.cluster, clusteropts.region)
			if err != nil {
				return err
			}
			return client.ComposeUp(context.Background(), project)
		}),
	}
	cmd.Flags().StringVar(&opts.loadBalancerArn, "load-balancer", "", "")
	return cmd
}

type downOptions struct {
	DeleteCluster bool
}

func DownCommand(clusteropts *clusterOptions, projectOpts *compose.ProjectOptions) *cobra.Command {
	opts := downOptions{}
	cmd := &cobra.Command{
		Use: "down",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := amazon.NewClient(clusteropts.profile, clusteropts.cluster, clusteropts.region)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				project, err := compose.ProjectFromOptions(projectOpts)
				if err != nil {
					return err
				}
				return client.ComposeDown(context.Background(), project.Name, opts.DeleteCluster)
			}
			// project names passed as parameters
			for _, name := range args {
				err := client.ComposeDown(context.Background(), name, opts.DeleteCluster)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.DeleteCluster, "delete-cluster", false, "Delete cluster")
	return cmd
}
