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

type attachOpts struct {
	*composeOptions

	service string
	index   int

	detachKeys string

	// todo docker container attach also has:
	// --no-stdin  // whole point of attach is for STDIN (can already attach to STDOUT and STDERR via docker compose up)
	// --sig-proxy

}

func attachCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := attachOpts{
		composeOptions: &composeOptions{
			ProjectOptions: p,
		},
	}
	runCmd := &cobra.Command{
		Use:   "attach [OPTIONS] SERVICE",
		Short: "Attach local standard input, output, and error streams to a service's running container.",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			opts.service = args[0]
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runAttach(ctx, dockerCli, backend, opts)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	runCmd.Flags().IntVar(&opts.index, "index", 0, "index of the container if service has multiple replicas.")
	runCmd.Flags().StringVarP(&opts.detachKeys, "detach-keys", "", "", "Override the key sequence for detaching from a container.")

	return runCmd
}

func runAttach(ctx context.Context, dockerCli command.Cli, backend api.Service, opts attachOpts) error {
	projectName, err := opts.toProjectName(dockerCli)
	if err != nil {
		return err
	}

	attachOpts := api.AttachOptions{
		Service:    opts.service,
		Index:      opts.index,
		DetachKeys: opts.detachKeys,
	}
	return backend.Attach(ctx, projectName, attachOpts)
}
