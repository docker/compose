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

	"github.com/morikuni/aec"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

type pullOptions struct {
	*projectOptions
	composeOptions
	quiet              bool
	parallel           bool
	noParallel         bool
	includeDeps        bool
	ignorePullFailures bool
}

func pullCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := pullOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "pull [SERVICE...]",
		Short: "Pull service images",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.noParallel {
				fmt.Fprint(os.Stderr, aec.Apply("option '--no-parallel' is DEPRECATED and will be ignored.\n", aec.RedF))
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runPull(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Pull without printing progress information")
	cmd.Flags().BoolVar(&opts.includeDeps, "include-deps", false, "Also pull services declared as dependencies")
	cmd.Flags().BoolVar(&opts.parallel, "parallel", true, "DEPRECATED pull multiple images in parallel.")
	flags.MarkHidden("parallel") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.parallel, "no-parallel", true, "DEPRECATED disable parallel pulling.")
	flags.MarkHidden("no-parallel") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.ignorePullFailures, "ignore-pull-failures", false, "Pull what it can and ignores images with pull failures")
	return cmd
}

func runPull(ctx context.Context, backend api.Service, opts pullOptions, services []string) error {
	project, err := opts.toProject(services)
	if err != nil {
		return err
	}

	if !opts.includeDeps {
		enabled, err := project.GetServices(services...)
		if err != nil {
			return err
		}
		for _, s := range project.Services {
			if !utils.StringContains(services, s.Name) {
				project.DisabledServices = append(project.DisabledServices, s)
			}
		}
		project.Services = enabled
	}

	return backend.Pull(ctx, project, api.PullOptions{
		Quiet:          opts.quiet,
		IgnoreFailures: opts.ignorePullFailures,
	})
}
