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

package context

import (
	"github.com/docker/compose-cli/cli/mobycli"
	"github.com/spf13/cobra"
)

// Command manages contexts
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage contexts",
	}

	cmd.AddCommand(
		createCommand(),
		listCommand(),
		removeCommand(),
		showCommand(),
		useCommand(),
		inspectCommand(),
		updateCommand(),
		exportCommand(),
		importCommand(),
	)

	return cmd
}

func exportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a context to a tar or kubeconfig file",
		Run: func(cmd *cobra.Command, args []string) {
			mobycli.Exec(cmd.Root())
		},
	}
	cmd.Flags().Bool("kubeconfig", false, "Export as a kubeconfig file")
	return cmd
}

func importCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a context from a tar or zip file",
		Run: func(cmd *cobra.Command, args []string) {
			mobycli.Exec(cmd.Root())
		},
	}
	return cmd
}
