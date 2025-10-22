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
	"os"

	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/metadata"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/cmdtrace"
	"github.com/docker/compose/v2/cmd/prompt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/compatibility"
	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/internal"
	"github.com/docker/compose/v2/pkg/compose"
)

func pluginMain() {
	plugin.Run(
		func(cli command.Cli) *cobra.Command {
			backend := compose.NewComposeService(cli,
				compose.WithPrompt(prompt.NewPrompt(cli.In(), cli.Out()).Confirm),
			)
			cmd := commands.RootCommand(cli, backend)
			originalPreRunE := cmd.PersistentPreRunE
			cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
				// initialize the cli instance
				if err := plugin.PersistentPreRunE(cmd, args); err != nil {
					return err
				}
				if err := cmdtrace.Setup(cmd, cli, os.Args[1:]); err != nil {
					logrus.Debugf("failed to enable tracing: %v", err)
				}

				if originalPreRunE != nil {
					return originalPreRunE(cmd, args)
				}
				return nil
			}

			cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
				return dockercli.StatusError{
					StatusCode: 1,
					Status:     err.Error(),
				}
			})
			return cmd
		},
		metadata.Metadata{
			SchemaVersion: "0.1.0",
			Vendor:        "Docker Inc.",
			Version:       internal.Version,
		},
		command.WithUserAgent("compose/"+internal.Version),
	)
}

func main() {
	if plugin.RunningStandalone() {
		os.Args = append([]string{"docker"}, compatibility.Convert(os.Args[1:])...)
	}
	pluginMain()
}
