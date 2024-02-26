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
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type restartOptions struct {
	*ProjectOptions
	timeChanged bool
	timeout     int
	noDeps      bool
}

func restartCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := restartOptions{
		ProjectOptions: p,
	}
	restartCmd := &cobra.Command{
		Use:   "restart [OPTIONS] [SERVICE...]",
		Short: "Restart service containers",
		PreRun: func(cmd *cobra.Command, args []string) {
			opts.timeChanged = cmd.Flags().Changed("timeout")
		},
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runRestart(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := restartCmd.Flags()
	flags.IntVarP(&opts.timeout, "timeout", "t", 0, "Specify a shutdown timeout in seconds")
	flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't restart dependent services")

	return restartCmd
}

func runRestart(ctx context.Context, dockerCli command.Cli, backend api.Service, opts restartOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli)
	if err != nil {
		return err
	}

	if project != nil && len(services) > 0 {
		project, err = project.WithServicesEnabled(services...)
		if err != nil {
			return err
		}
	}

	var timeout *time.Duration
	if opts.timeChanged {
		timeoutValue := time.Duration(opts.timeout) * time.Second
		timeout = &timeoutValue
	}

	return backend.Restart(ctx, name, api.RestartOptions{
		Timeout:  timeout,
		Services: services,
		Project:  project,
		NoDeps:   opts.noDeps,
	})
}
