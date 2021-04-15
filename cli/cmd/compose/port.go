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

	"github.com/docker/compose-cli/api/compose"
)

type portOptions struct {
	*projectOptions
	protocol string
	index    int
}

func portCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := portOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "port [options] [--] SERVICE PRIVATE_PORT",
		Short: "Print the public port for a port binding.",
		Args:  cobra.MinimumNArgs(2),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			port, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			return runPort(ctx, backend, opts, args[0], port)
		}),
	}
	cmd.Flags().StringVar(&opts.protocol, "protocol", "tcp", "tcp or udp")
	cmd.Flags().IntVar(&opts.index, "index", 1, "index of the container if service has multiple replicas")
	return cmd
}

func runPort(ctx context.Context, backend compose.Service, opts portOptions, service string, port int) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	ip, port, err := backend.Port(ctx, projectName, service, port, compose.PortOptions{
		Protocol: opts.protocol,
		Index:    opts.index,
	})
	if err != nil {
		return err
	}

	fmt.Printf("%s:%d\n", ip, port)
	return nil
}
