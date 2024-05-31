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
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type portOptions struct {
	*ProjectOptions
	port     uint16
	protocol string
	index    int
}

func portCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := portOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "port [OPTIONS] SERVICE [PRIVATE_PORT]",
		Short: "List port mappings or print the public port of a specific mapping for the service",
		Args:  cobra.RangeArgs(1, 2),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if len(args) == 1 {
				opts.port = 0
				return nil
			}

			port, err := strconv.ParseUint(args[1], 10, 16)
			if err != nil {
				return err
			}
			opts.port = uint16(port)
			opts.protocol = strings.ToLower(opts.protocol)
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPort(ctx, dockerCli, backend, opts, args[0])
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	cmd.Flags().StringVar(&opts.protocol, "protocol", "tcp", "tcp or udp")
	cmd.Flags().IntVar(&opts.index, "index", 0, "Index of the container if service has multiple replicas")
	return cmd
}

func runPort(ctx context.Context, dockerCli command.Cli, backend api.Service, opts portOptions, service string) error {
	projectName, err := opts.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}

	ports, err := backend.Port(ctx, projectName, service, opts.port, api.PortOptions{
		Protocol: opts.protocol,
		Index:    opts.index,
	})
	if err != nil {
		return err
	}

	for _, port := range ports {
		fmt.Fprintf(dockerCli.Out(), "%s\n", port.String())
	}

	return nil
}
