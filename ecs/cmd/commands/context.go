package commands

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/docker/cli/cli/command"
	amazon "github.com/docker/ecs-plugin/pkg/amazon/backend"
	"github.com/docker/ecs-plugin/pkg/docker"
	"github.com/spf13/cobra"
)

type ContextFunc func(ctx docker.AwsContext, backend *amazon.Backend, args []string) error

func WithAwsContext(dockerCli command.Cli, f ContextFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx, err := docker.GetAwsContext(dockerCli)
		if err != nil {
			return err
		}
		backend, err := amazon.NewBackend(ctx.Profile, ctx.Region)
		if err != nil {
			return err
		}
		err = f(*ctx, backend, args)
		if e, ok := err.(awserr.Error); ok {
			return fmt.Errorf(e.Message())
		}
		return err
	}
}
