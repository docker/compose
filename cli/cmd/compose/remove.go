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

	"github.com/docker/compose-cli/api/client"
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

func removeCommand(p *projectOptions) *cobra.Command {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd.Context(), opts, args)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&opts.force, "force", "f", false, "Don't ask to confirm removal")
	f.BoolVarP(&opts.stop, "stop", "s", false, "Stop the containers, if required, before removing")
	f.BoolVarP(&opts.volumes, "volumes", "v", false, "Remove any anonymous volumes attached to containers")
	return cmd
}

func runRemove(ctx context.Context, opts removeOptions, services []string) error {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	if opts.stop {
		_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
			err := c.ComposeService().Stop(ctx, project, compose.StopOptions{})
			return "", err
		})
		if err != nil {
			return err
		}
	}

	reosurces, err := c.ComposeService().Remove(ctx, project, compose.RemoveOptions{
		DryRun: true,
	})
	if err != nil {
		return err
	}

	if len(reosurces) == 0 {
		fmt.Println("No stopped containers")
		return nil
	}
	msg := fmt.Sprintf("Going to remove %s", strings.Join(reosurces, ", "))
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

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		_, err = c.ComposeService().Remove(ctx, project, compose.RemoveOptions{
			Volumes: opts.volumes,
			Force:   opts.force,
		})
		return "", err
	})
	return err
}
