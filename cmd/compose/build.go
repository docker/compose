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

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type buildOptions struct {
	*projectOptions
	composeOptions
	quiet    bool
	pull     bool
	progress string
	args     []string
	noCache  bool
	memory   string
}

func buildCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := buildOptions{
		projectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "build [SERVICE...]",
		Short: "Build or rebuild services",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.memory != "" {
				fmt.Println("WARNING --memory is ignored as not supported in buildkit.")
			}
			if opts.quiet {
				devnull, err := os.Open(os.DevNull)
				if err != nil {
					return err
				}
				os.Stdout = devnull
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runBuild(ctx, backend, opts, args)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Don't print anything to STDOUT")
	cmd.Flags().BoolVar(&opts.pull, "pull", false, "Always attempt to pull a newer version of the image.")
	cmd.Flags().StringVar(&opts.progress, "progress", "auto", `Set type of progress output ("auto", "plain", "noTty")`)
	cmd.Flags().StringArrayVar(&opts.args, "build-arg", []string{}, "Set build-time variables for services.")
	cmd.Flags().Bool("parallel", true, "Build images in parallel. DEPRECATED")
	cmd.Flags().MarkHidden("parallel") //nolint:errcheck
	cmd.Flags().Bool("compress", true, "Compress the build context using gzip. DEPRECATED")
	cmd.Flags().MarkHidden("compress") //nolint:errcheck
	cmd.Flags().Bool("force-rm", true, "Always remove intermediate containers. DEPRECATED")
	cmd.Flags().MarkHidden("force-rm") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false, "Do not use cache when building the image")
	cmd.Flags().Bool("no-rm", false, "Do not remove intermediate containers after a successful build. DEPRECATED")
	cmd.Flags().MarkHidden("no-rm") //nolint:errcheck
	cmd.Flags().StringVarP(&opts.memory, "memory", "m", "", "Set memory limit for the build container. Not supported on buildkit yet.")
	cmd.Flags().MarkHidden("memory") //nolint:errcheck

	return cmd
}

func runBuild(ctx context.Context, backend api.Service, opts buildOptions, services []string) error {
	project, err := opts.toProject(services, cli.WithResolvedPaths(true))
	if err != nil {
		return err
	}

	return backend.Build(ctx, project, api.BuildOptions{
		Pull:     opts.pull,
		Progress: opts.progress,
		Args:     types.NewMappingWithEquals(opts.args),
		NoCache:  opts.noCache,
		Quiet:    opts.quiet,
		Services: services,
	})
}
