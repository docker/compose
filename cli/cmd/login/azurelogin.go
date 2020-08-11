package login

import (
	"github.com/spf13/cobra"

	"github.com/docker/api/aci"
)

// AzureLoginCommand returns the azure login command
func AzureLoginCommand() *cobra.Command {
	opts := aci.LoginParams{}
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Log in to azure",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.Validate(); err != nil {
				return err
			}
			return cloudLogin(cmd, "aci", opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.TenantID, "tenant-id", "", "Specify tenant ID to use")
	flags.StringVar(&opts.ClientID, "client-id", "", "Client ID for Service principal login")
	flags.StringVar(&opts.ClientSecret, "client-secret", "", "Client secret for Service principal login")

	return cmd
}
