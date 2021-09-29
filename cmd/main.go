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
	"fmt"
	"os"

	dockercli "github.com/docker/cli/cli"
	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli-plugins/plugin"
	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	cliFlags "github.com/docker/cli/cli/flags"
	commands "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/internal"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

func init() {
	commands.Warning = "The new 'docker compose' command is currently experimental. " +
		"To provide feedback or request new features please open issues at https://github.com/docker/compose"
}

func standaloneMain() int {
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	opts := cliFlags.NewClientOptions()
	err = dockerCli.Initialize(opts)
	if err != nil {
		return 1
	}
	lazyInit := api.NewServiceProxy().WithService(compose.NewComposeService(dockerCli.Client(), dockerCli.ConfigFile()))
	rootCmd := commands.RootCommand(lazyInit)
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func pluginMain() {
	plugin.Run(func(dockerCli command.Cli) *cobra.Command {
		lazyInit := api.NewServiceProxy()
		cmd := commands.RootCommand(lazyInit)
		originalPreRun := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := plugin.PersistentPreRunE(cmd, args); err != nil {
				return err
			}
			lazyInit.WithService(compose.NewComposeService(dockerCli.Client(), dockerCli.ConfigFile()))
			if originalPreRun != nil {
				return originalPreRun(cmd, args)
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

func main() {
	if commands.RunningAsStandalone() {
		os.Exit(standaloneMain())
	}
	pluginMain()
}
