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
	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	*ProjectOptions
	force   bool
	stop    bool
	volumes bool
}

func removeCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Compose) *cobra.Command {
	opts := removeOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "rm [OPTIONS] [SERVICE...]",
		Short: "Removes stopped service containers",
		Long: `Removes stopped service containers

By default, anonymous volumes attached to containers will not be removed. You
can override this with -v. To list all volumes, use "docker volume ls".

Any data which is not in a volume will be lost.`,
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runRemove(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	f := cmd.Flags()
	f.BoolVarP(&opts.force, "force", "f", false, "Don't ask to confirm removal")
	f.BoolVarP(&opts.stop, "stop", "s", false, "Stop the containers, if required, before removing")
	f.BoolVarP(&opts.volumes, "volumes", "v", false, "Remove any anonymous volumes attached to containers")
	f.BoolP("all", "a", false, "Deprecated - no effect")
	f.MarkHidden("all") //nolint:errcheck

	return cmd
}

func runRemove(ctx context.Context, dockerCli command.Cli, backend api.Compose, opts removeOptions, services []string) error {
	project, name, err := opts.projectOrName(ctx, dockerCli, services...)
	if err != nil {
		return err
	}

	return backend.Remove(ctx, name, api.RemoveOptions{
		Services: services,
		Force:    opts.force,
		Volumes:  opts.volumes,
		Project:  project,
		Stop:     opts.stop,
	})
}
