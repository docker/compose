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

	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

func killCommand(p *projectOptions, backend api.Service) *cobra.Command {
	var opts api.KillOptions
	cmd := &cobra.Command{
		Use:   "kill [options] [SERVICE...]",
		Short: "Force stop service containers.",
		RunE: p.WithProject(func(ctx context.Context, project *types.Project) error {
			return backend.Kill(ctx, project, opts)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.Signal, "signal", "s", "SIGKILL", "SIGNAL to send to the container.")

	return cmd
}
