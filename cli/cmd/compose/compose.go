package compose

import (
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/compose"
)

func Command() *cobra.Command {
	command := &cobra.Command{
		Short: "Docker Compose",
		Use:   "compose",
	}
	command.AddCommand(
		upCommand(),
		downCommand(),
	)
	return command
}

func upCommand() *cobra.Command {
	opts := &compose.ProjectOptions{}
	upCmd := &cobra.Command{
		Use: "up",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return err
			}
			return c.AciService().Up(cmd.Context(), *opts)
		},
	}
	upCmd.Flags().StringVar(&opts.Name, "name",  "", "Project name")
	upCmd.Flags().StringVar(&opts.WorkDir, "workdir",  ".", "Work dir")
	upCmd.Flags().StringArrayVarP(&opts.ConfigPaths, "file", "f", []string{}, "Compose configuration files")
	upCmd.Flags().StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")

	return upCmd
}

func downCommand() *cobra.Command {
	opts := &compose.ProjectOptions{}
	downCmd := &cobra.Command{
		Use: "down",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(cmd.Context())
			if err != nil {
				return err
			}
			return c.AciService().Down(cmd.Context(), *opts)
		},
	}
	downCmd.Flags().StringVar(&opts.Name, "name",  "", "Project name")
	downCmd.Flags().StringVar(&opts.WorkDir, "workdir",  ".", "Work dir")

	return downCmd
}