package context

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	cliconfig "github.com/docker/api/cli/config"
	cliopts "github.com/docker/api/cli/options"
	"github.com/docker/api/context/store"
)

func useCommand(opts *cliopts.GlobalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "use CONTEXT",
		Short: "Set the default context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUse(cmd.Context(), opts.Config, args[0])
		},
	}
}

func runUse(ctx context.Context, configDir string, name string) error {
	s := store.ContextStore(ctx)
	// Match behavior of existing CLI
	if name != store.DefaultContextName {
		if _, err := s.Get(name, nil); err != nil {
			return err
		}
	}
	if err := cliconfig.WriteCurrentContext(configDir, name); err != nil {
		return err
	}
	fmt.Println(name)
	return nil
}
