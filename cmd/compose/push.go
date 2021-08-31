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

type pushOptions struct {
	*projectOptions
	composeOptions

	Ignorefailures bool
}

func pushCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := pushOptions{
		projectOptions: p,
	}
	pushCmd := &cobra.Command{
		Use:   "push [SERVICE...]",
		Short: "Push service images",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPush(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	pushCmd.Flags().BoolVar(&opts.Ignorefailures, "ignore-push-failures", false, "Push what it can and ignores images with push failures")

	return pushCmd
}

func runPush(ctx context.Context, backend api.Service, opts pushOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	return backend.Push(ctx, project, api.PushOptions{
		IgnoreFailures: opts.Ignorefailures,
	})
}
