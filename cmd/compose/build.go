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
	"strings"

	"github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	buildx "github.com/docker/buildx/util/progress"
	cliopts "github.com/docker/cli/opts"
	ui "github.com/docker/compose/v2/pkg/progress"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type buildOptions struct {
	*ProjectOptions
	composeOptions
	quiet   bool
	pull    bool
	push    bool
	args    []string
	noCache bool
	memory  cliopts.MemBytes
	ssh     string
	builder string
}

func (opts buildOptions) toAPIBuildOptions(services []string) (api.BuildOptions, error) {
	var SSHKeys []types.SSHKey
	var err error
	if opts.ssh != "" {
		SSHKeys, err = loader.ParseShortSSHSyntax(opts.ssh)
		if err != nil {
			return api.BuildOptions{}, err
		}
	}
	builderName := opts.builder
	if builderName == "" {
		builderName = os.Getenv("BUILDX_BUILDER")
	}

	return api.BuildOptions{
		Pull:     opts.pull,
		Push:     opts.push,
		Progress: ui.Mode,
		Args:     types.NewMappingWithEquals(opts.args),
		NoCache:  opts.noCache,
		Quiet:    opts.quiet,
		Services: services,
		SSHs:     SSHKeys,
		Builder:  builderName,
	}, nil
}

func buildCommand(p *ProjectOptions, progress *string, backend api.Service) *cobra.Command {
	opts := buildOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "build [OPTIONS] [SERVICE...]",
		Short: "Build or rebuild services",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.quiet {
				ui.Mode = ui.ModeQuiet
				devnull, err := os.Open(os.DevNull)
				if err != nil {
					return err
				}
				os.Stdout = devnull
			}
			return nil
		}),
		RunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("ssh") && opts.ssh == "" {
				opts.ssh = "default"
			}
			if cmd.Flags().Changed("progress") && opts.ssh == "" {
				fmt.Fprint(os.Stderr, "--progress is a global compose flag, better use `docker compose --progress xx build ...")
			}
			return runBuild(ctx, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	cmd.Flags().BoolVar(&opts.push, "push", false, "Push service images.")
	cmd.Flags().BoolVarP(&opts.quiet, "quiet", "q", false, "Don't print anything to STDOUT")
	cmd.Flags().BoolVar(&opts.pull, "pull", false, "Always attempt to pull a newer version of the image.")
	cmd.Flags().StringArrayVar(&opts.args, "build-arg", []string{}, "Set build-time variables for services.")
	cmd.Flags().StringVar(&opts.ssh, "ssh", "", "Set SSH authentications used when building service images. (use 'default' for using your default SSH Agent)")
	cmd.Flags().StringVar(&opts.builder, "builder", "", "Set builder to use.")
	cmd.Flags().Bool("parallel", true, "Build images in parallel. DEPRECATED")
	cmd.Flags().MarkHidden("parallel") //nolint:errcheck
	cmd.Flags().Bool("compress", true, "Compress the build context using gzip. DEPRECATED")
	cmd.Flags().MarkHidden("compress") //nolint:errcheck
	cmd.Flags().Bool("force-rm", true, "Always remove intermediate containers. DEPRECATED")
	cmd.Flags().MarkHidden("force-rm") //nolint:errcheck
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false, "Do not use cache when building the image")
	cmd.Flags().Bool("no-rm", false, "Do not remove intermediate containers after a successful build. DEPRECATED")
	cmd.Flags().MarkHidden("no-rm") //nolint:errcheck
	cmd.Flags().VarP(&opts.memory, "memory", "m", "Set memory limit for the build container. Not supported by BuildKit.")
	cmd.Flags().StringVar(progress, "progress", buildx.PrinterModeAuto, fmt.Sprintf(`Set type of ui output (%s)`, strings.Join(printerModes, ", ")))
	cmd.Flags().MarkHidden("progress") //nolint:errcheck

	return cmd
}

func runBuild(ctx context.Context, backend api.Service, opts buildOptions, services []string) error {
	project, err := opts.ToProject(services, cli.WithResolvedPaths(true))
	if err != nil {
		return err
	}

	apiBuildOptions, err := opts.toAPIBuildOptions(services)
	if err != nil {
		return err
	}

	apiBuildOptions.Memory = int64(opts.memory)
	return backend.Build(ctx, project, apiBuildOptions)
}
