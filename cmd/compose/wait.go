/*
   Copyright 2023 Docker Compose CLI authors

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

	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type waitOptions struct {
	*ProjectOptions

	services []string

	downProject bool
}

func waitCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := waitOptions{
		ProjectOptions: p,
	}

	var statusCode int64
	var err error
	cmd := &cobra.Command{
		Use:   "wait SERVICE [SERVICE...] [OPTIONS]",
		Short: "Block until containers of all (or specified) services stop.",
		Args:  cli.RequiresMinArgs(1),
		RunE: Adapt(func(ctx context.Context, services []string) error {
			opts.services = services
			statusCode, err = runWait(ctx, dockerCli, backend, &opts)
			return err
		}),
		PostRun: func(cmd *cobra.Command, args []string) {
			os.Exit(int(statusCode))
		},
	}

	cmd.Flags().BoolVar(&opts.downProject, "down-project", false, "Drops project when the first container stops")

	return cmd
}

func runWait(ctx context.Context, dockerCli command.Cli, backend api.Service, opts *waitOptions) (int64, error) {
	_, name, err := opts.projectOrName(ctx, dockerCli)
	if err != nil {
		return 0, err
	}

	return backend.Wait(ctx, name, api.WaitOptions{
		Services:                   opts.services,
		DownProjectOnContainerExit: opts.downProject,
	})
}
