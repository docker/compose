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

package main

import (
	"strings"

	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	api "github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/cmd/compose"
	"github.com/docker/compose-cli/cli/metrics"
	"github.com/docker/compose-cli/internal"
	impl "github.com/docker/compose-cli/local/compose"
)

func main() {
	plugin.Run(func(dockerCli command.Cli) *cobra.Command {
		lazyInit := api.ServiceDelegator{
			Delegate: api.NoImpl{},
		}
		cmd := compose.RootCommand(store.DefaultContextType, &lazyInit)
		originalPreRun := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := plugin.PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			lazyInit.Delegate = impl.NewComposeService(dockerCli.Client(), dockerCli.ConfigFile())
			if originalPreRun != nil {
				return originalPreRun(cmd, args)
			}
			return nil
		}
		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return dockercli.StatusError{
				StatusCode: metrics.CommandSyntaxFailure.ExitCode,
				Status:     err.Error(),
			}
		})
		return cmd
	},
		manager.Metadata{
			SchemaVersion: "0.1.0",
			Vendor:        "Docker Inc.",
			Version:       strings.TrimPrefix(internal.Version, "v"),
		})
}
