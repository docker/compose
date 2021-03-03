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

package login

import (
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/aci"
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
	flags.StringVar(&opts.CloudName, "cloud-name", "", "Name of a registered Azure cloud [AzureCloud | AzureChinaCloud | AzureGermanCloud | AzureUSGovernment] (AzureCloud by default)")

	return cmd
}
