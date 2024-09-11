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
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"

	"github.com/docker/cli/cli/command"
	cliformatter "github.com/docker/cli/cli/command/formatter"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/spf13/cobra"
)

type psOptions struct {
	*ProjectOptions
	Format   string
	All      bool
	Quiet    bool
	Services bool
	Filter   string
	Status   []string
	noTrunc  bool
	Orphans  bool
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
		p.Status = append(p.Status, parts[1])
	case "source":
		return api.ErrNotImplemented
	default:
		return fmt.Errorf("unknown filter %s", parts[0])
	}
	return nil
}

func psCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := psOptions{
		ProjectOptions: p,
	}
	psCmd := &cobra.Command{
		Use:   "ps [OPTIONS] [SERVICE...]",
		Short: "List containers",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return opts.parseFilter()
		},
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPs(ctx, dockerCli, backend, args, opts)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := psCmd.Flags()
	flags.StringVar(&opts.Format, "format", "table", cliflags.FormatHelp)
	flags.StringVar(&opts.Filter, "filter", "", "Filter services by a property (supported filters: status)")
	flags.StringArrayVar(&opts.Status, "status", []string{}, "Filter services by status. Values: [paused | restarting | removing | running | dead | created | exited]")
	flags.BoolVarP(&opts.Quiet, "quiet", "q", false, "Only display IDs")
	flags.BoolVar(&opts.Services, "services", false, "Display services")
	flags.BoolVar(&opts.Orphans, "orphans", true, "Include orphaned services (not declared by project)")
	flags.BoolVarP(&opts.All, "all", "a", false, "Show all stopped containers (including those created by the run command)")
	flags.BoolVar(&opts.noTrunc, "no-trunc", false, "Don't truncate output")
	return psCmd
}

func runPs(ctx context.Context, dockerCli command.Cli, backend api.Service, services []string, opts psOptions) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	if project != nil {
		names := project.ServiceNames()
		if len(services) > 0 {
			for _, service := range services {
				if !utils.StringContains(names, service) {
					return fmt.Errorf("no such service: %s", service)
				}
			}
		} else if !opts.Orphans {
			// until user asks to list orphaned services, we only include those declared in project
			services = names
		}
	}

	containers, err := backend.Ps(ctx, name, api.PsOptions{
		Project:  project,
		All:      opts.All || len(opts.Status) != 0,
		Services: services,
	})
	if err != nil {
		return err
	}

	if len(opts.Status) != 0 {
		containers = filterByStatus(containers, opts.Status)
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	if opts.Quiet {
		for _, c := range containers {
			_, _ = fmt.Fprintln(dockerCli.Out(), c.ID)
		}
		return nil
	}

	if opts.Services {
		services := []string{}
		for _, c := range containers {
			s := c.Service
			if !utils.StringContains(services, s) {
				services = append(services, s)
			}
		}
		_, _ = fmt.Fprintln(dockerCli.Out(), strings.Join(services, "\n"))
		return nil
	}

	if opts.Format == "" {
		opts.Format = dockerCli.ConfigFile().PsFormat
	}

	containerCtx := cliformatter.Context{
		Output: dockerCli.Out(),
		Format: formatter.NewContainerFormat(opts.Format, opts.Quiet, false),
		Trunc:  !opts.noTrunc,
	}
	return formatter.ContainerWrite(containerCtx, containers)
}

func filterByStatus(containers []api.ContainerSummary, statuses []string) []api.ContainerSummary {
	var filtered []api.ContainerSummary
	for _, c := range containers {
		if hasStatus(c, statuses) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func hasStatus(c api.ContainerSummary, statuses []string) bool {
	for _, status := range statuses {
		if c.State == status {
			return true
		}
	}
	return false
}
