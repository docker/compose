package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command"
	amazon "github.com/docker/ecs-plugin/pkg/amazon/backend"
	"github.com/docker/ecs-plugin/pkg/docker"
	"github.com/spf13/cobra"
)

func ComposeCommand(dockerCli command.Cli) *cobra.Command {
	cmd := &cobra.Command{
		Use: "compose",
	}
	opts := &cli.ProjectOptions{}
	AddFlags(opts, cmd.Flags())

	cmd.AddCommand(
		ConvertCommand(dockerCli, opts),
		UpCommand(dockerCli, opts),
		DownCommand(dockerCli, opts),
		LogsCommand(dockerCli, opts),
		PsCommand(dockerCli, opts),
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

func ConvertCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "convert",
		RunE: WithProject(projectOpts, func(project *types.Project, args []string) error {
			clusteropts, err := docker.GetAwsContext(dockerCli)
			if err != nil {
				return err
			}
			backend, err := amazon.NewBackend(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			template, err := backend.Convert(project)
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

func UpCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "up",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			backend, err := amazon.NewBackend(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			return backend.Up(context.Background(), *projectOpts)
		}),
	}
	cmd.Flags().StringVar(&opts.loadBalancerArn, "load-balancer", "", "")
	return cmd
}

func PsCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "ps",
		RunE: WithProject(projectOpts, func(project *types.Project, args []string) error {
			clusteropts, err := docker.GetAwsContext(dockerCli)
			if err != nil {
				return err
			}
			backend, err := amazon.NewBackend(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			status, err := backend.Ps(context.Background(), project)
			if err != nil {
				return err
			}
			printSection(os.Stdout, len(status), func(w io.Writer) {
				for _, service := range status {
					fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", service.ID, service.Name, service.Replicas, service.Desired, strings.Join(service.Ports, " "))
				}
			}, "ID", "NAME", "REPLICAS", "PORTS")
			return nil
		}),
	}
	cmd.Flags().StringVar(&opts.loadBalancerArn, "load-balancer", "", "")
	return cmd
}

type downOptions struct {
	DeleteCluster bool
}

func DownCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	opts := downOptions{}
	cmd := &cobra.Command{
		Use: "down",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			backend, err := amazon.NewBackend(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			return backend.Down(context.Background(), *projectOpts)
		}),
	}
	cmd.Flags().BoolVar(&opts.DeleteCluster, "delete-cluster", false, "Delete cluster")
	return cmd
}

func LogsCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "logs [PROJECT NAME]",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, args []string) error {
			backend, err := amazon.NewBackend(clusteropts.Profile, clusteropts.Cluster, clusteropts.Region)
			if err != nil {
				return err
			}
			var name string

			if len(args) == 0 {
				project, err := cli.ProjectFromOptions(projectOpts)
				if err != nil {
					return err
				}
				name = project.Name
			} else {
				name = args[0]
			}
			return backend.Logs(context.Background(), name)
		}),
	}
	return cmd
}
