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

	"github.com/docker/compose/v2/pkg/api"
)

type watchOptions struct {
	*projectOptions
}

func watchCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := watchOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "watch [SERVICES...]",
		Short: "Watch build context for service(s) and rebuild/refresh containers when files are updated",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runWatch(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}

	return cmd
}

func runWatch(ctx context.Context, backend api.Service, opts watchOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	return backend.Watch(ctx, project, api.WatchOptions{})
}
