/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package logout

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/errdefs"
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
