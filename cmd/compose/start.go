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
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/spf13/cobra"
)

type startOptions struct {
	*ProjectOptions
	wait        bool
	waitTimeout int
}

func startCommand(p *ProjectOptions, dockerCli command.Cli, backendOptions *BackendOptions) *cobra.Command {
	opts := startOptions{
		ProjectOptions: p,
	}
	startCmd := &cobra.Command{
		Use:   "start [SERVICE...]",
		Short: "Start services",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runStart(ctx, dockerCli, backendOptions, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := startCmd.Flags()
	flags.BoolVar(&opts.wait, "wait", false, "Wait for services to be running|healthy. Implies detached mode.")
	flags.IntVar(&opts.waitTimeout, "wait-timeout", 0, "Maximum duration in seconds to wait for the project to be running|healthy")

	return startCmd
}

func runStart(ctx context.Context, dockerCli command.Cli, backendOptions *BackendOptions, opts startOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	backend, err := compose.NewComposeService(dockerCli, backendOptions.Options...)
	if err != nil {
		return err
	}

	timeout := time.Duration(opts.waitTimeout) * time.Second
	return backend.Start(ctx, name, api.StartOptions{
		AttachTo:    services,
		Project:     project,
		Services:    services,
		Wait:        opts.wait,
		WaitTimeout: timeout,
	})
}
