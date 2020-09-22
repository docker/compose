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
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/cli/mobycli"
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
	mobycli.Exec(cmd.Root())
	return nil
}
