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

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/pkg/api"
)

type logsOptions struct {
	*ProjectOptions
	composeOptions
	follow     bool
	index      int
	tail       string
	since      string
	until      string
	noColor    bool
	noPrefix   bool
	timestamps bool
}

func logsCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := logsOptions{
		ProjectOptions: p,
	}
	logsCmd := &cobra.Command{
		Use:   "logs [OPTIONS] [SERVICE...]",
		Short: "View output from containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runLogs(ctx, dockerCli, backend, opts, args)
		}),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.index > 0 && len(args) != 1 {
				return errors.New("--index requires one service to be selected")
			}
			return nil
		},
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := logsCmd.Flags()
	flags.BoolVarP(&opts.follow, "follow", "f", false, "Follow log output")
	flags.IntVar(&opts.index, "index", 0, "index of the container if service has multiple replicas")
	flags.StringVar(&opts.since, "since", "", "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	flags.StringVar(&opts.until, "until", "", "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	flags.BoolVar(&opts.noColor, "no-color", false, "Produce monochrome output")
	flags.BoolVar(&opts.noPrefix, "no-log-prefix", false, "Don't print prefix in logs")
	flags.BoolVarP(&opts.timestamps, "timestamps", "t", false, "Show timestamps")
	flags.StringVarP(&opts.tail, "tail", "n", "all", "Number of lines to show from the end of the logs for each container")
	return logsCmd
}

func runLogs(ctx context.Context, dockerCli command.Cli, backend api.Service, opts logsOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	// exclude services configured to ignore output (attach: false), until explicitly selected
	if project != nil && len(services) == 0 {
		for n, service := range project.Services {
			if service.Attach == nil || *service.Attach {
				services = append(services, n)
			}
		}
	}

	consumer := formatter.NewLogConsumer(ctx, dockerCli.Out(), dockerCli.Err(), !opts.noColor, !opts.noPrefix, false)
	return backend.Logs(ctx, name, consumer, api.LogOptions{
		Project:    project,
		Services:   services,
		Follow:     opts.follow,
		Index:      opts.index,
		Tail:       opts.tail,
		Since:      opts.since,
		Until:      opts.until,
		Timestamps: opts.timestamps,
	})
}
