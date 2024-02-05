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

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type startOptions struct {
	*ProjectOptions
}

func startCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := startOptions{
		ProjectOptions: p,
	}
	startCmd := &cobra.Command{
		Use:   "start [SERVICE...]",
		Short: "Start services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runStart(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	return startCmd
}

func runStart(ctx context.Context, dockerCli command.Cli, backend api.Service, opts startOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	return backend.Start(ctx, name, api.StartOptions{
		AttachTo: services,
		Project:  project,
		Services: services,
	})
}
