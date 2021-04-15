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
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/cli/formatter"
	"github.com/docker/compose-cli/utils"
)

type psOptions struct {
	*projectOptions
	Format   string
	All      bool
	Quiet    bool
	Services bool
}

func psCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := psOptions{
		projectOptions: p,
	}
	psCmd := &cobra.Command{
		Use:   "ps",
		Short: "List containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPs(ctx, backend, opts)
		}),
	}
	psCmd.Flags().StringVar(&opts.Format, "format", "pretty", "Format the output. Values: [pretty | json].")
	psCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	psCmd.Flags().BoolVar(&opts.Services, "services", false, "Display services")
	psCmd.Flags().BoolVarP(&opts.All, "all", "a", false, "Show all stopped containers (including those created by the run command)")
	return psCmd
}

func runPs(ctx context.Context, backend compose.Service, opts psOptions) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	containers, err := backend.Ps(ctx, projectName, compose.PsOptions{
		All: opts.All,
	})
	if err != nil {
		return err
	}

	if opts.Services {
		services := []string{}
		for _, s := range containers {
			if !utils.StringContains(services, s.Service) {
				services = append(services, s.Service)
			}
		}
		fmt.Println(strings.Join(services, "\n"))
		return nil
	}
	if opts.Quiet {
		for _, s := range containers {
			fmt.Println(s.ID)
		}
		return nil
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	return formatter.Print(containers, opts.Format, os.Stdout,
		func(w io.Writer) {
			for _, container := range containers {
				var ports []string
				for _, p := range container.Publishers {
					if p.URL == "" {
						ports = append(ports, fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol))
					} else {
						ports = append(ports, fmt.Sprintf("%s->%d/%s", p.URL, p.TargetPort, p.Protocol))
					}
				}
				status := container.State
				if container.Health != "" {
					status = fmt.Sprintf("%s (%s)", container.State, container.Health)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", container.Name, container.Service, status, strings.Join(ports, ", "))
			}
		},
		"NAME", "SERVICE", "STATUS", "PORTS")
}
