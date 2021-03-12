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

	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
)

type killOptions struct {
	*projectOptions
	Signal string
}

func killCommand(p *projectOptions) *cobra.Command {
	opts := killOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "kill [options] [SERVICE...]",
		Short: "Force stop service containers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(cmd.Context(), opts, args)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.Signal, "signal", "s", "SIGKILL", "SIGNAL to send to the container.")

	return cmd
}

func runKill(ctx context.Context, opts killOptions, services []string) error {
	c, err := client.New(ctx)
	if err != nil {
		return err
	}
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}
	return c.ComposeService().Kill(ctx, project, compose.KillOptions{
		Signal: opts.Signal,
	})
}
