package logout

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/api/client"
	"github.com/docker/api/errdefs"
)

// AzureLogoutCommand returns the azure logout command
func AzureLogoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Logout from Azure",
		Args:  cobra.MaximumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cloudLogout(cmd, "aci")
		},
	}
	return cmd
}

func cloudLogout(cmd *cobra.Command, backendType string) error {
	ctx := cmd.Context()
	cs, err := client.GetCloudService(ctx, backendType)
	if err != nil {
		return errors.Wrap(errdefs.ErrLoginFailed, "cannot connect to backend")
	}
	err = cs.Logout(ctx)
	if errors.Is(err, context.Canceled) {
		return errors.New("logout canceled")
	}
	if err != nil {
		return err
	}
	fmt.Println("Removing login credentials for Azure")
	return nil
}
