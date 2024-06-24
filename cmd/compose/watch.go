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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v2/cmd/formatter"

	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/internal/locker"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type watchOptions struct {
	*ProjectOptions
	prune bool
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
		Short: "Watch build context for service and rebuild/refresh containers when files are updated",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			if cmd.Parent().Name() == "alpha" {
				logrus.Warn("watch command is now available as a top level command")
			}
			return runWatch(ctx, dockerCli, backend, watchOpts, buildOpts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}

	cmd.Flags().BoolVar(&buildOpts.quiet, "quiet", false, "hide build output")
	cmd.Flags().BoolVar(&watchOpts.prune, "prune", false, "Prune dangling images on rebuild")
	cmd.Flags().BoolVar(&watchOpts.noUp, "no-up", false, "Do not build & start services before watching")
	return cmd
}

func runWatch(ctx context.Context, dockerCli command.Cli, backend api.Service, watchOpts watchOptions, buildOpts buildOptions, services []string) error {
	project, _, err := watchOpts.ToProject(ctx, dockerCli, nil)
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
		return fmt.Errorf("cannot take exclusive lock for project %q: %w", project.Name, err)
	}
	if err := l.Lock(); err != nil {
		return fmt.Errorf("cannot take exclusive lock for project %q: %w", project.Name, err)
	}

	if !watchOpts.noUp {
		for index, service := range project.Services {
			if service.Build != nil && service.Develop != nil {
				service.PullPolicy = types.PullPolicyBuild
			}
			project.Services[index] = service
		}
		upOpts := api.UpOptions{
			Create: api.CreateOptions{
				Build:                &build,
				Services:             services,
				RemoveOrphans:        false,
				Recreate:             api.RecreateDiverged,
				RecreateDependencies: api.RecreateNever,
				Inherit:              true,
				QuietPull:            buildOpts.quiet,
			},
			Start: api.StartOptions{
				Project:  project,
				Attach:   nil,
				Services: services,
			},
		}
		if err := backend.Up(ctx, project, upOpts); err != nil {
			return err
		}
	}

	consumer := formatter.NewLogConsumer(ctx, dockerCli.Out(), dockerCli.Err(), false, false, false)
	return backend.Watch(ctx, project, services, api.WatchOptions{
		Build: &build,
		LogTo: consumer,
		Prune: watchOpts.prune,
	})
}
