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
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

type killOptions struct {
	*projectOptions
	removeOrphans bool
	signal        string
}

func killCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := killOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "kill [OPTIONS] [SERVICE...]",
		Short: "Force stop service containers.",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runKill(ctx, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}

	flags := cmd.Flags()
	removeOrphans := utils.StringToBool(os.Getenv("COMPOSE_REMOVE_ORPHANS"))
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", removeOrphans, "Remove containers for services not defined in the Compose file.")
	flags.StringVarP(&opts.signal, "signal", "s", "SIGKILL", "SIGNAL to send to the container.")

	return cmd
}

func runKill(ctx context.Context, backend api.Service, opts killOptions, services []string) error {
	project, name, err := opts.projectOrName()
	if err != nil {
		return err
	}

	return backend.Kill(ctx, name, api.KillOptions{
		RemoveOrphans: opts.removeOrphans,
		Project:       project,
		Services:      services,
		Signal:        opts.signal,
	})

}
