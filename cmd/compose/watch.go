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

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type watchOptions struct {
	*ProjectOptions
	quiet bool
}

func watchCommand(p *ProjectOptions, backend api.Service) *cobra.Command {
	opts := watchOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "watch [SERVICE...]",
		Short: "EXPERIMENTAL - Watch build context for service and rebuild/refresh containers when files are updated",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runWatch(ctx, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}

	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "hide build output")
	return cmd
}

func runWatch(ctx context.Context, backend api.Service, opts watchOptions, services []string) error {
	fmt.Fprintln(os.Stderr, "watch command is EXPERIMENTAL")
	project, err := opts.ToProject(nil)
	if err != nil {
		return err
	}

	return backend.Watch(ctx, project, services, api.WatchOptions{})
}
