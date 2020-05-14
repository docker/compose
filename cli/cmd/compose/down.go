package compose

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/compose"
)

func downCommand() *cobra.Command {
	opts := compose.ProjectOptions{}
	downCmd := &cobra.Command{
		Use: "down",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(cmd.Context(), opts)
		},
	}
	downCmd.Flags().StringVar(&opts.Name, "name", "", "Project name")
	downCmd.Flags().StringVar(&opts.WorkDir, "workdir", ".", "Work dir")
	downCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")

	return downCmd
}

func runDown(ctx context.Context, opts compose.ProjectOptions) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}

	composeService := c.ComposeService()
	if composeService == nil {
		return errors.New("compose not implemented in current context")
	}

	return composeService.Down(ctx, opts)
}
