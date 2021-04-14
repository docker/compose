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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
)

type createOptions struct {
	*composeOptions
	forceRecreate bool
	noRecreate    bool
}

func createCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := createOptions{
		composeOptions: &composeOptions{},
	}
	cmd := &cobra.Command{
		Use:   "create [SERVICE...]",
		Short: "Creates containers for a service.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Build && opts.noBuild {
				return fmt.Errorf("--build and --no-build are incompatible")
			}
			if opts.forceRecreate && opts.noRecreate {
				return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
			}
			return runCreateStart(cmd.Context(), backend, upOptions{
				composeOptions: &composeOptions{
					projectOptions: p,
					Build:          opts.Build,
					noBuild:        opts.noBuild,
				},
				noStart:       true,
				forceRecreate: opts.forceRecreate,
				noRecreate:    opts.noRecreate,
			}, args)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&opts.noBuild, "no-build", false, "Don't build an image, even if it's missing.")
	flags.BoolVar(&opts.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	flags.BoolVar(&opts.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	return cmd
}
