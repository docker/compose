package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/ecs-plugin/pkg/amazon/cloudformation"

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
	opts := &composeOptions{}
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

func ConvertCommand(dockerCli command.Cli, options *composeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "convert",
		RunE: WithAwsContext(dockerCli, func(ctx docker.AwsContext, backend *amazon.Backend, args []string) error {
			opts, err := options.toProjectOptions()
			if err != nil {
				return err
			}
			project, err := cli.ProjectFromOptions(opts)
			if err != nil {
				return err
			}
			template, err := backend.Convert(project)
			if err != nil {
				return err
			}

			json, err := cloudformation.Marshall(template)
			if err != nil {
				fmt.Printf("Failed to generate JSON: %s\n", err)
			} else {
				fmt.Printf("%s\n", string(json))
			}
			return nil
		}),
	}
	return cmd
}

func UpCommand(dockerCli command.Cli, options *composeOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "up",
		RunE: WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			opts, err := options.toProjectOptions()
			if err != nil {
				return err
			}
			return backend.Up(context.Background(), opts)
		}),
	}
	cmd.Flags().StringVar(&opts.loadBalancerArn, "load-balancer", "", "")
	return cmd
}

func PsCommand(dockerCli command.Cli, options *composeOptions) *cobra.Command {
	opts := upOptions{}
	cmd := &cobra.Command{
		Use: "ps",
		RunE: WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			opts, err := options.toProjectOptions()
			if err != nil {
				return err
			}
			status, err := backend.Ps(context.Background(), opts)
			if err != nil {
				return err
			}
			printSection(os.Stdout, len(status), func(w io.Writer) {
				for _, service := range status {
					fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\n", service.ID, service.Name, service.Replicas, service.Desired, strings.Join(service.Ports, ", "))
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

func DownCommand(dockerCli command.Cli, options *composeOptions) *cobra.Command {
	opts := downOptions{}
	cmd := &cobra.Command{
		Use: "down",
		RunE: WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			opts, err := options.toProjectOptions()
			if err != nil {
				return err
			}
			return backend.Down(context.Background(), opts)
		}),
	}
	cmd.Flags().BoolVar(&opts.DeleteCluster, "delete-cluster", false, "Delete cluster")
	return cmd
}

func LogsCommand(dockerCli command.Cli, options *composeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "logs [PROJECT NAME]",
		RunE: WithAwsContext(dockerCli, func(clusteropts docker.AwsContext, backend *amazon.Backend, args []string) error {
			opts, err := options.toProjectOptions()
			if err != nil {
				return err
			}
			return backend.Logs(context.Background(), opts)
		}),
	}
	return cmd
}
