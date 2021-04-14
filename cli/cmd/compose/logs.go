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

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/cli/formatter"
)

type logsOptions struct {
	*projectOptions
	composeOptions
	follow     bool
	tail       string
	noColor    bool
	noPrefix   bool
	timestamps bool
}

func logsCommand(p *projectOptions, contextType string, backend compose.Service) *cobra.Command {
	opts := logsOptions{
		projectOptions: p,
	}
	logsCmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "View output from containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd.Context(), backend, opts, args)
		},
	}
	flags := logsCmd.Flags()
	flags.BoolVarP(&opts.follow, "follow", "f", false, "Follow log output.")
	flags.BoolVar(&opts.noColor, "no-color", false, "Produce monochrome output.")
	flags.BoolVar(&opts.noPrefix, "no-log-prefix", false, "Don't print prefix in logs.")
	flags.BoolVarP(&opts.timestamps, "timestamps", "t", false, "Show timestamps.")

	if contextType == store.DefaultContextType {
		flags.StringVar(&opts.tail, "tail", "all", "Number of lines to show from the end of the logs for each container.")
	}
	return logsCmd
}

func runLogs(ctx context.Context, backend compose.Service, opts logsOptions, services []string) error {
	projectName, err := opts.toProjectName()
	if err != nil {
		return err
	}
	consumer := formatter.NewLogConsumer(ctx, os.Stdout, !opts.noColor, !opts.noPrefix)
	return backend.Logs(ctx, projectName, consumer, compose.LogOptions{
		Services:   services,
		Follow:     opts.follow,
		Tail:       opts.tail,
		Timestamps: opts.timestamps,
	})
}
