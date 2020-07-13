package login

import (
	"github.com/spf13/cobra"

	"github.com/docker/api/azure"
)

// AzureLoginCommand returns the azure login command
func AzureLoginCommand() *cobra.Command {
	opts := azure.LoginParams{}
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Log in to azure",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cloudLogin(cmd, "aci", opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.TenantID, "tenant-id", "", "Specify tenant ID to use from your azure account")

	return cmd
}
