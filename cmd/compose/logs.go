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
	"os"

	"github.com/docker/compose/v2/cmd/formatter"

	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type logsOptions struct {
	*projectOptions
	composeOptions
	follow     bool
	tail       string
	since      string
	until      string
	noColor    bool
	noPrefix   bool
	timestamps bool
}

func logsCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := logsOptions{
		projectOptions: p,
	}
	logsCmd := &cobra.Command{
		Use:   "logs [SERVICE...]",
		Short: "View output from containers",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runLogs(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := logsCmd.Flags()
	flags.BoolVarP(&opts.follow, "follow", "f", false, "Follow log output.")
	flags.StringVar(&opts.since, "since", "", "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	flags.StringVar(&opts.until, "until", "", "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	flags.BoolVar(&opts.noColor, "no-color", false, "Produce monochrome output.")
	flags.BoolVar(&opts.noPrefix, "no-log-prefix", false, "Don't print prefix in logs.")
	flags.BoolVarP(&opts.timestamps, "timestamps", "t", false, "Show timestamps.")
	flags.StringVar(&opts.tail, "tail", "all", "Number of lines to show from the end of the logs for each container.")
	return logsCmd
}

func runLogs(ctx context.Context, backend api.Service, opts logsOptions, services []string) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	consumer := formatter.NewLogConsumer(ctx, os.Stdout, !opts.noColor, !opts.noPrefix)
	return backend.Logs(ctx, projectName, consumer, api.LogOptions{
		Services:   services,
		Follow:     opts.follow,
		Tail:       opts.tail,
		Since:      opts.since,
		Until:      opts.until,
		Timestamps: opts.timestamps,
	})
}
