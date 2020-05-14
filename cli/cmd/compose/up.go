package compose

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/compose"
)

func upCommand() *cobra.Command {
	opts := compose.ProjectOptions{}
	upCmd := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd.Context(), opts)
		},
	}
	upCmd.Flags().StringVar(&opts.Name, "name", "", "Project name")
	upCmd.Flags().StringVar(&opts.WorkDir, "workdir", ".", "Work dir")
	upCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	upCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")

	return upCmd
}

func runUp(ctx context.Context, opts compose.ProjectOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	composeService := c.ComposeService()
	if composeService == nil {
		return errors.New("compose not implemented in current context")
	}

	return composeService.Up(ctx, opts)
}
