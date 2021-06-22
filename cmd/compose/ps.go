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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/cli/formatter"
	"github.com/docker/compose-cli/pkg/api"
	"github.com/docker/compose-cli/pkg/utils"
)

type psOptions struct {
	*projectOptions
	Format   string
	All      bool
	Quiet    bool
	Services bool
	Filter   string
	Status   string
}

func (p *psOptions) parseFilter() error {
	if p.Filter == "" {
		return nil
	}
	parts := strings.SplitN(p.Filter, "=", 2)
	if len(parts) != 2 {
		return errors.New("arguments to --filter should be in form KEY=VAL")
	}
	switch parts[0] {
	case "status":
		p.Status = parts[1]
	case "source":
		return api.ErrNotImplemented
	default:
		return fmt.Errorf("unknow filter %s", parts[0])
	}
	return nil
}

func psCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := psOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List containers",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return opts.parseFilter()
		},
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPs(ctx, backend, args, opts)
		}),
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Format, "format", "pretty", "Format the output. Values: [pretty | json]")
	flags.StringVar(&opts.Filter, "filter", "", "Filter services by a property")
	flags.StringVar(&opts.Status, "status", "", "Filter services by status")
	flags.BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	flags.BoolVar(&opts.Services, "services", false, "Display services")
	flags.BoolVarP(&opts.All, "all", "a", false, "Show all stopped containers (including those created by the run command)")
	flags.Lookup("filter").Hidden = true
	return cmd
}

func runPs(ctx context.Context, backend api.Service, services []string, opts psOptions) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	containers, err := backend.Ps(ctx, projectName, api.PsOptions{
		All:      opts.All,
		Services: services,
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

	if opts.Status != "" {
		containers = filterByStatus(containers, opts.Status)
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
				if status == "running" && container.Health != "" {
					status = fmt.Sprintf("%s (%s)", container.State, container.Health)
				} else if status == "exited" || status == "dead" {
					status = fmt.Sprintf("%s (%d)", container.State, container.ExitCode)
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", container.Name, container.Service, status, strings.Join(ports, ", "))
			}
		},
		"NAME", "SERVICE", "STATUS", "PORTS")
}

func filterByStatus(containers []api.ContainerSummary, status string) []api.ContainerSummary {
	hasContainerWithState := map[string]struct{}{}
	for _, c := range containers {
		if c.State == status {
			hasContainerWithState[c.Service] = struct{}{}
		}
	}
	var filtered []api.ContainerSummary
	for _, c := range containers {
		if _, ok := hasContainerWithState[c.Service]; ok {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
