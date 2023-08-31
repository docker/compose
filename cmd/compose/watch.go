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
	"os"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/internal/locker"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type watchOptions struct {
	*ProjectOptions
	quiet bool
	noUp  bool
}

func watchCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	watchOpts := watchOptions{
		ProjectOptions: p,
	}
	buildOpts := buildOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "watch [SERVICE...]",
		Short: "EXPERIMENTAL - Watch build context for service and rebuild/refresh containers when files are updated",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runWatch(ctx, dockerCli, backend, watchOpts, buildOpts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	cmd.Flags().BoolVar(&watchOpts.quiet, "quiet", false, "hide build output")
	cmd.Flags().BoolVar(&watchOpts.noUp, "no-up", false, "Do not build & start services before watching")
	return cmd
}

func runWatch(ctx context.Context, dockerCli command.Cli, backend api.Service, watchOpts watchOptions, buildOpts buildOptions, services []string) error {
	fmt.Fprintln(os.Stderr, "watch command is EXPERIMENTAL")
	project, err := watchOpts.ToProject(dockerCli, nil)
	if err != nil {
		return err
	}

	if err := applyPlatforms(project, true); err != nil {
		return err
	}

	build, err := buildOpts.toAPIBuildOptions(nil)
	if err != nil {
		return err
	}

	// validation done -- ensure we have the lockfile for this project before doing work
	l, err := locker.NewPidfile(project.Name)
	if err != nil {
		return fmt.Errorf("cannot take exclusive lock for project %q: %v", project.Name, err)
	}
	if err := l.Lock(); err != nil {
		return fmt.Errorf("cannot take exclusive lock for project %q: %v", project.Name, err)
	}

	if !watchOpts.noUp {
		upOpts := api.UpOptions{
			Create: api.CreateOptions{
				Build:                &build,
				Services:             services,
				RemoveOrphans:        false,
				Recreate:             api.RecreateDiverged,
				RecreateDependencies: api.RecreateNever,
				Inherit:              true,
				QuietPull:            watchOpts.quiet,
			},
			Start: api.StartOptions{
				Project:     project,
				Attach:      nil,
				CascadeStop: false,
				Services:    services,
			},
		}
		if err := backend.Up(ctx, project, upOpts); err != nil {
			return err
		}
	}
	return backend.Watch(ctx, project, services, api.WatchOptions{
		Build: build,
	})
}
