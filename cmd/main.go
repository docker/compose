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
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/cmdtrace"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/compatibility"
	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/internal"
	"github.com/docker/compose/v2/pkg/compose"
)

func pluginMain() {
	plugin.Run(func(dockerCli command.Cli) *cobra.Command {
		// TODO(milas): this cast is safe but we should not need to do this,
		// 	we should expose the concrete service type so that we do not need
		// 	to rely on the `api.Service` interface internally
		backend := compose.NewComposeService(dockerCli).(commands.Backend)
		cmd := commands.RootCommand(dockerCli, backend)
		originalPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			// initialize the dockerCli instance
			if err := plugin.PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			// compose-specific initialization
			dockerCliPostInitialize(dockerCli)

			if err := cmdtrace.Setup(cmd, dockerCli, os.Args[1:]); err != nil {
				logrus.Debugf("failed to enable tracing: %v", err)
			}

			if originalPreRunE != nil {
				return originalPreRunE(cmd, args)
			}
			return nil
		}

		cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
			return dockercli.StatusError{
				StatusCode: compose.CommandSyntaxFailure.ExitCode,
				Status:     err.Error(),
			}
		})
		return cmd
	},
		manager.Metadata{
			SchemaVersion: "0.1.0",
			Vendor:        "Docker Inc.",
			Version:       internal.Version,
		})
}

// dockerCliPostInitialize performs Compose-specific configuration for the
// command.Cli instance provided by the plugin.Run() initialization.
//
// NOTE: This must be called AFTER plugin.PersistentPreRunE.
func dockerCliPostInitialize(dockerCli command.Cli) {
	// HACK(milas): remove once docker/cli#4574 is merged; for now,
	// set it in a rather roundabout way by grabbing the underlying
	// concrete client and manually invoking an option on it
	_ = dockerCli.Apply(func(cli *command.DockerCli) error {
		if mobyClient, ok := cli.Client().(*client.Client); ok {
			_ = client.WithUserAgent("compose/" + internal.Version)(mobyClient)
		}
		return nil
	})
}

func main() {
	if plugin.RunningStandalone() {
		os.Args = append([]string{"docker"}, compatibility.Convert(os.Args[1:])...)
	}
	pluginMain()
}
