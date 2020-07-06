package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/compose-spec/compose-go/cli"
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

func ConvertCommand(dockerCli command.Cli, options *cli.ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "convert",
		RunE: docker.WithAwsContext(dockerCli, func(ctx docker.AwsContext, backend *amazon.Backend, args []string) error {
			project, err := cli.ProjectFromOptions(options)
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

func UpCommand(dockerCli command.Cli, options *cli.ProjectOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "up",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			return backend.Up(context.Background(), *options)
		}),
	}
	cmd.Flags().StringVar(&opts.loadBalancerArn, "load-balancer", "", "")
	return cmd
}

func PsCommand(dockerCli command.Cli, options *cli.ProjectOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "ps",
		RunE: docker.WithAwsContext(dockerCli, func(ctx docker.AwsContext, backend *amazon.Backend, args []string) error {
			status, err := backend.Ps(context.Background(), *options)
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
		RunE: docker.WithAwsContext(dockerCli, func(ctx docker.AwsContext, backend *amazon.Backend, args []string) error {
			return backend.Down(context.Background(), *projectOpts)
		}),
	}
	cmd.Flags().BoolVar(&opts.DeleteCluster, "delete-cluster", false, "Delete cluster")
	return cmd
}

func LogsCommand(dockerCli command.Cli, projectOpts *cli.ProjectOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "logs [PROJECT NAME]",
		RunE: docker.WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			return backend.Logs(context.Background(), *projectOpts)
		}),
	}
	return cmd
}
