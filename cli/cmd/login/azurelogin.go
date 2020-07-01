package login

import (
	"github.com/spf13/cobra"

	"github.com/docker/api/azure/login"
)

type azureLoginOpts struct {
	tenantID string
}

// AzureLoginCommand returns the azure login command
func AzureLoginCommand() *cobra.Command {
	opts := azureLoginOpts{}
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Log in to azure",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cloudLogin(cmd, "aci", map[string]string{login.TenantIDLoginParam: opts.tenantID})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.tenantID, "tenant-id", "", "Specify tenant ID to use from your azure account")

	return cmd
}
