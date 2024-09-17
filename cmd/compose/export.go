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

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type exportOptions struct {
	*ProjectOptions

	service string
	output  string
	index   int
}

func exportCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	options := exportOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "export [OPTIONS] SERVICE",
		Short: "Export a service container's filesystem as a tar archive",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			options.service = args[0]
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runExport(ctx, dockerCli, backend, options)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	flags := cmd.Flags()
	flags.IntVar(&options.index, "index", 0, "index of the container if service has multiple replicas.")
	flags.StringVarP(&options.output, "output", "o", "", "Write to a file, instead of STDOUT")

	return cmd
}

func runExport(ctx context.Context, dockerCli command.Cli, backend api.Service, options exportOptions) error {
	projectName, err := options.toProjectName(ctx, dockerCli)
	if err != nil {
		return err
	}

	exportOptions := api.ExportOptions{
		Service: options.service,
		Index:   options.index,
		Output:  options.output,
	}

	return backend.Export(ctx, projectName, exportOptions)
}
