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
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type pauseOptions struct {
	*ProjectOptions
}

func pauseCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := pauseOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pause [SERVICE...]",
		Short: "Pause services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPause(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	return cmd
}

func runPause(ctx context.Context, dockerCli command.Cli, backend api.Service, opts pauseOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	return backend.Pause(ctx, name, api.PauseOptions{
		Services: services,
		Project:  project,
	})
}

type unpauseOptions struct {
	*ProjectOptions
}

func unpauseCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := unpauseOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "unpause [SERVICE...]",
		Short: "Unpause services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runUnPause(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	return cmd
}

func runUnPause(ctx context.Context, dockerCli command.Cli, backend api.Service, opts unpauseOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	return backend.UnPause(ctx, name, api.PauseOptions{
		Services: services,
		Project:  project,
	})
}
