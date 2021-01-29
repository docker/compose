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
	"github.com/spf13/cobra"
)

type createOptions struct {
	*composeOptions
}

func createCommand(p *projectOptions) *cobra.Command {
	opts := createOptions{
		composeOptions: &composeOptions{},
	}
	cmd := &cobra.Command{
		Use:   "create [SERVICE...]",
		Short: "Creates containers for a service.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateStart(cmd.Context(), upOptions{
				composeOptions: &composeOptions{
					projectOptions: p,
					Build:          opts.Build,
				},
				noStart: true,
			}, args)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers.")
	return cmd
}
