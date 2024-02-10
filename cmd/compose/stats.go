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

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type statsOptions struct {
	ProjectOptions *ProjectOptions
	all            bool
	format         string
	noStream       bool
	noTrunc        bool
}

func statsCommand(p *ProjectOptions, dockerCli command.Cli) *cobra.Command {
	opts := statsOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "stats [OPTIONS] [SERVICE]",
		Short: "Display a live stream of container(s) resource usage statistics",
		Args:  cobra.MaximumNArgs(1),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runStats(ctx, dockerCli, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&opts.all, "all", "a", false, "Show all containers (default shows just running)")
	flags.StringVar(&opts.format, "format", "", `Format output using a custom template:
'table':            Print output in table format with column headers (default)
'table TEMPLATE':   Print output in table format using the given Go template
'json':             Print in JSON format
'TEMPLATE':         Print output using the given Go template.
Refer to https://docs.docker.com/go/formatting/ for more information about formatting output with templates`)
	flags.BoolVar(&opts.noStream, "no-stream", false, "Disable streaming stats and only pull the first result")
	flags.BoolVar(&opts.noTrunc, "no-trunc", false, "Do not truncate output")
	return cmd
}

func runStats(ctx context.Context, dockerCli command.Cli, opts statsOptions, service []string) error {
	name, err := opts.ProjectOptions.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}
	filter := []filters.KeyValuePair{
		filters.Arg("label", fmt.Sprintf("%s=%s", api.ProjectLabel, name)),
	}
	if len(service) > 0 {
		filter = append(filter, filters.Arg("label", fmt.Sprintf("%s=%s", api.ServiceLabel, service[0])))
	}
	args := filters.NewArgs(filter...)
	return container.RunStats(ctx, dockerCli, &container.StatsOptions{
		All:      opts.all,
		NoStream: opts.noStream,
		NoTrunc:  opts.noTrunc,
		Format:   opts.format,
		Filters:  &args,
	})
}
