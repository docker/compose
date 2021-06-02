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
	"strings"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/utils/prompt"

	"github.com/spf13/cobra"
)

type removeOptions struct {
	*projectOptions
	force   bool
	stop    bool
	volumes bool
}

func removeCommand(p *projectOptions, backend compose.Service) *cobra.Command {
	opts := removeOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "rm [SERVICE...]",
		Short: "Removes stopped service containers",
		Long: `Removes stopped service containers

By default, anonymous volumes attached to containers will not be removed. You
can override this with -v. To list all volumes, use "docker volume ls".

Any data which is not in a volume will be lost.`,
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runRemove(ctx, backend, opts, args)
		}),
	}
	f := cmd.Flags()
	f.BoolVarP(&opts.force, "force", "f", false, "Don't ask to confirm removal")
	f.BoolVarP(&opts.stop, "stop", "s", false, "Stop the containers, if required, before removing")
	f.BoolVarP(&opts.volumes, "volumes", "v", false, "Remove any anonymous volumes attached to containers")
	f.BoolP("all", "a", false, "Deprecated - no effect")
	f.MarkHidden("all") //nolint:errcheck

	return cmd
}

func runRemove(ctx context.Context, backend compose.Service, opts removeOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	if opts.stop {
		err = progress.Run(ctx, func(ctx context.Context) error {
			return backend.Stop(ctx, project, compose.StopOptions{
				Services: services,
			})
		})
		if err != nil {
			return err
		}
	}

	resources, err := backend.Remove(ctx, project, compose.RemoveOptions{
		DryRun:   true,
		Services: services,
	})
	if err != nil {
		return err
	}

	if len(resources) == 0 {
		fmt.Println("No stopped containers")
		return nil
	}
	msg := fmt.Sprintf("Going to remove %s", strings.Join(resources, ", "))
	if opts.force {
		fmt.Println(msg)
	} else {
		confirm, err := prompt.User{}.Confirm(msg, false)
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	return progress.Run(ctx, func(ctx context.Context) error {
		_, err := backend.Remove(ctx, project, compose.RemoveOptions{
			Volumes: opts.volumes,
			Force:   opts.force,
		})
		return err
	})
}
