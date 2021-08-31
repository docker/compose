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

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type portOptions struct {
	*projectOptions
	port     int
	protocol string
	index    int
}

func portCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := portOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "port [options] [--] SERVICE PRIVATE_PORT",
		Short: "Print the public port for a port binding.",
		Args:  cobra.MinimumNArgs(2),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			port, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			opts.port = port
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPort(ctx, backend, opts, args[0])
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	cmd.Flags().StringVar(&opts.protocol, "protocol", "tcp", "tcp or udp")
	cmd.Flags().IntVar(&opts.index, "index", 1, "index of the container if service has multiple replicas")
	return cmd
}

func runPort(ctx context.Context, backend api.Service, opts portOptions, service string) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	ip, port, err := backend.Port(ctx, projectName, service, opts.port, api.PortOptions{
		Protocol: opts.protocol,
		Index:    opts.index,
	})
	if err != nil {
		return err
	}

	fmt.Printf("%s:%d\n", ip, port)
	return nil
}
