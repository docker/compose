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

package compose

import (
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

// alphaCommand groups all experimental subcommands
func alphaCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	cmd := &cobra.Command{
		Short:  "Experimental commands",
		Use:    "alpha [COMMAND]",
		Hidden: true,
		Annotations: map[string]string{
			"experimentalCLI": "true",
		},
	}
	cmd.AddCommand(
		vizCommand(p, dockerCli, backend),
		publishCommand(p, dockerCli, backend),
		generateCommand(p, backend),
	)
	return cmd
}
