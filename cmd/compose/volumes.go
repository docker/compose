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
	"slices"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/command/formatter"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type volumesOptions struct {
	*ProjectOptions
	Quiet  bool
	Format string
}

func volumesCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	options := volumesOptions{
		ProjectOptions: p,
	}

	cmd := &cobra.Command{
		Use:   "volumes [OPTIONS] [SERVICE...]",
		Short: "List volumes",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runVol(ctx, dockerCli, backend, args, options)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Only display volume names")
	cmd.Flags().StringVar(&options.Format, "format", "table", flags.FormatHelp)

	return cmd
}

func runVol(ctx context.Context, dockerCli command.Cli, backend api.Service, services []string, options volumesOptions) error {
	project, _, err := options.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	names := project.ServiceNames()

	if len(services) == 0 {
		services = names
	}

	for _, service := range services {
		if !slices.Contains(names, service) {
			return fmt.Errorf("no such service: %s", service)
		}
	}

	volumes, err := backend.Volumes(ctx, project, api.VolumesOptions{
		Services: services,
	})
	if err != nil {
		return err
	}

	if options.Quiet {
		for _, v := range volumes {
			_, _ = fmt.Fprintln(dockerCli.Out(), v.Name)
		}
		return nil
	}

	volumeCtx := formatter.Context{
		Output: dockerCli.Out(),
		Format: formatter.NewVolumeFormat(options.Format, options.Quiet),
	}

	return formatter.VolumeWrite(volumeCtx, volumes)
}
