package logout

import (
	"github.com/spf13/cobra"

	"github.com/docker/api/cli/mobycli"
)

// Command returns the login command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout [SERVER]",
		Short: "Log out from a Docker registry or cloud backend",
		Long:  "Log out from a Docker registry or cloud backend.\nIf no server is specified, the default is defined by the daemon.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runLogout,
	}

	cmd.AddCommand(AzureLogoutCommand())
	return cmd
}

func runLogout(cmd *cobra.Command, args []string) error {
	mobycli.Exec()
	return nil
}
