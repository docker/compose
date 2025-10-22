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
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type stopOptions struct {
	*ProjectOptions
	timeChanged bool
	timeout     int
}

func stopCommand(p *ProjectOptions, dockerCli command.Cli, backendOptions []compose.Option) *cobra.Command {
	opts := stopOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "stop [OPTIONS] [SERVICE...]",
		Short: "Stop services",
		PreRun: func(cmd *cobra.Command, args []string) {
			opts.timeChanged = cmd.Flags().Changed("timeout")
		},
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runStop(ctx, dockerCli, backendOptions, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.IntVarP(&opts.timeout, "timeout", "t", 0, "Specify a shutdown timeout in seconds")

	return cmd
}

func runStop(ctx context.Context, dockerCli command.Cli, backendOptions []compose.Option, opts stopOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	var timeout *time.Duration
	if opts.timeChanged {
		timeoutValue := time.Duration(opts.timeout) * time.Second
		timeout = &timeoutValue
	}

	backend, err := compose.NewComposeService(dockerCli, backendOptions...)
	if err != nil {
		return err
	}

	return backend.Stop(ctx, name, api.StopOptions{
		Timeout:  timeout,
		Services: services,
		Project:  project,
	})
}
