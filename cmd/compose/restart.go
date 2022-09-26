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

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type restartOptions struct {
	*projectOptions
	timeout int
}

func restartCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := restartOptions{
		projectOptions: p,
	}
	restartCmd := &cobra.Command{
		Use:   "restart [OPTIONS] [SERVICE...]",
		Short: "Restart service containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runRestart(ctx, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	flags := restartCmd.Flags()
	flags.IntVarP(&opts.timeout, "timeout", "t", 10, "Specify a shutdown timeout in seconds")

	return restartCmd
}

func runRestart(ctx context.Context, backend api.Service, opts restartOptions, services []string) error {
	project, name, err := opts.projectOrName()
	if err != nil {
		return err
	}

	timeout := time.Duration(opts.timeout) * time.Second
	return backend.Restart(ctx, name, api.RestartOptions{
		Timeout:  &timeout,
		Services: services,
		Project:  project,
	})
}
