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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/morikuni/aec"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type pullOptions struct {
	*ProjectOptions
	composeOptions
	quiet              bool
	parallel           bool
	noParallel         bool
	includeDeps        bool
	ignorePullFailures bool
	noBuildable        bool
	policy             string
}

func pullCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := pullOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pull [OPTIONS] [SERVICE...]",
		Short: "Pull service images",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.noParallel {
				fmt.Fprint(os.Stderr, aec.Apply("option '--no-parallel' is DEPRECATED and will be ignored.\n", aec.RedF))
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPull(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Pull without printing progress information")
	cmd.Flags().BoolVar(&opts.includeDeps, "include-deps", false, "Also pull services declared as dependencies")
	cmd.Flags().BoolVar(&opts.parallel, "parallel", true, "DEPRECATED pull multiple images in parallel")
	flags.MarkHidden("parallel") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.parallel, "no-parallel", true, "DEPRECATED disable parallel pulling")
	flags.MarkHidden("no-parallel") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.ignorePullFailures, "ignore-pull-failures", false, "Pull what it can and ignores images with pull failures")
	cmd.Flags().BoolVar(&opts.noBuildable, "ignore-buildable", false, "Ignore images that can be built")
	cmd.Flags().StringVar(&opts.policy, "policy", "", `Apply pull policy ("missing"|"always")`)
	return cmd
}

func (opts pullOptions) apply(project *types.Project, services []string) (*types.Project, error) {
	if !opts.includeDeps {
		var err error
		project, err = project.WithSelectedServices(services, types.IgnoreDependencies)
		if err != nil {
			return nil, err
		}
	}

	if opts.policy != "" {
		for i, service := range project.Services {
			if service.Image == "" {
				continue
			}
			service.PullPolicy = opts.policy
			project.Services[i] = service
		}
	}
	return project, nil
}

func runPull(ctx context.Context, dockerCli command.Cli, backend api.Service, opts pullOptions, services []string) error {
	project, _, err := opts.ToProject(ctx, dockerCli, services)
	if err != nil {
		return err
	}

	project, err = opts.apply(project, services)
	if err != nil {
		return err
	}

	return backend.Pull(ctx, project, api.PullOptions{
		Quiet:           opts.quiet,
		IgnoreFailures:  opts.ignorePullFailures,
		IgnoreBuildable: opts.noBuildable,
	})
}
