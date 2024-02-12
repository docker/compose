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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type pushOptions struct {
	*ProjectOptions
	composeOptions
	IncludeDeps    bool
	Ignorefailures bool
	Quiet          bool
}

func pushCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := pushOptions{
		ProjectOptions: p,
	}
	pushCmd := &cobra.Command{
		Use:   "push [OPTIONS] [SERVICE...]",
		Short: "Push service images",
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPush(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	pushCmd.Flags().BoolVar(&opts.Ignorefailures, "ignore-push-failures", false, "Push what it can and ignores images with push failures")
	pushCmd.Flags().BoolVar(&opts.IncludeDeps, "include-deps", false, "Also push images of services declared as dependencies")
	pushCmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "Push without printing progress information")

	return pushCmd
}

func runPush(ctx context.Context, dockerCli command.Cli, backend api.Service, opts pushOptions, services []string) error {
	project, _, err := opts.ToProject(ctx, dockerCli, services)
	if err != nil {
		return err
	}

	if !opts.IncludeDeps {
		project, err = project.WithSelectedServices(services, types.IgnoreDependencies)
		if err != nil {
			return err
		}
	}

	return backend.Push(ctx, project, api.PushOptions{
		IgnoreFailures: opts.Ignorefailures,
		Quiet:          opts.Quiet,
	})
}
