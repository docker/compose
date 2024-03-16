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

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	cliopts "github.com/docker/cli/opts"
	ui "github.com/docker/compose/v2/pkg/progress"
	buildkit "github.com/moby/buildkit/util/progress/progressui"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type buildOptions struct {
	*ProjectOptions
	quiet   bool
	pull    bool
	push    bool
	args    []string
	noCache bool
	memory  cliopts.MemBytes
	ssh     string
	builder string
	deps    bool
}

func (opts buildOptions) toAPIBuildOptions(services []string) (api.BuildOptions, error) {
	var SSHKeys []types.SSHKey
	var err error
	if opts.ssh != "" {
		id, path, found := strings.Cut(opts.ssh, "=")
		if !found && id != "default" {
			return api.BuildOptions{}, fmt.Errorf("invalid ssh key %q", opts.ssh)
		}
		SSHKeys = append(SSHKeys, types.SSHKey{
			ID:   id,
			Path: path,
		})
		if err != nil {
			return api.BuildOptions{}, err
		}
	}
	builderName := opts.builder
	if builderName == "" {
		builderName = os.Getenv("BUILDX_BUILDER")
	}

	uiMode := ui.Mode
	if uiMode == ui.ModeJSON {
		uiMode = "rawjson"
	}
	return api.BuildOptions{
		Pull:     opts.pull,
		Push:     opts.push,
		Progress: uiMode,
		Args:     types.NewMappingWithEquals(opts.args),
		NoCache:  opts.noCache,
		Quiet:    opts.quiet,
		Services: services,
		Deps:     opts.deps,
		SSHs:     SSHKeys,
		Builder:  builderName,
	}, nil
}

func buildCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
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
				fmt.Fprint(os.Stderr, "--progress is a global compose flag, better use `docker compose --progress xx build ...\n")
			}
			return runBuild(ctx, dockerCli, backend, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.push, "push", false, "Push service images")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Don't print anything to STDOUT")
	flags.BoolVar(&opts.pull, "pull", false, "Always attempt to pull a newer version of the image")
	flags.StringArrayVar(&opts.args, "build-arg", []string{}, "Set build-time variables for services")
	flags.StringVar(&opts.ssh, "ssh", "", "Set SSH authentications used when building service images. (use 'default' for using your default SSH Agent)")
	flags.StringVar(&opts.builder, "builder", "", "Set builder to use")
	flags.BoolVar(&opts.deps, "with-dependencies", false, "Also build dependencies (transitively)")

	flags.Bool("parallel", true, "Build images in parallel. DEPRECATED")
	flags.MarkHidden("parallel") //nolint:errcheck
	flags.Bool("compress", true, "Compress the build context using gzip. DEPRECATED")
	flags.MarkHidden("compress") //nolint:errcheck
	flags.Bool("force-rm", true, "Always remove intermediate containers. DEPRECATED")
	flags.MarkHidden("force-rm") //nolint:errcheck
	flags.BoolVar(&opts.noCache, "no-cache", false, "Do not use cache when building the image")
	flags.Bool("no-rm", false, "Do not remove intermediate containers after a successful build. DEPRECATED")
	flags.MarkHidden("no-rm") //nolint:errcheck
	flags.VarP(&opts.memory, "memory", "m", "Set memory limit for the build container. Not supported by BuildKit.")
	flags.StringVar(&p.Progress, "progress", string(buildkit.AutoMode), fmt.Sprintf(`Set type of ui output (%s)`, strings.Join(printerModes, ", ")))
	flags.MarkHidden("progress") //nolint:errcheck

	return cmd
}

func runBuild(ctx context.Context, dockerCli command.Cli, backend api.Service, opts buildOptions, services []string) error {
	project, _, err := opts.ToProject(ctx, dockerCli, services, cli.WithResolvedPaths(true), cli.WithoutEnvironmentResolution)
	if err != nil {
		return err
	}

	if err := applyPlatforms(project, false); err != nil {
		return err
	}

	apiBuildOptions, err := opts.toAPIBuildOptions(services)
	if err != nil {
		return err
	}

	apiBuildOptions.Memory = int64(opts.memory)
	return backend.Build(ctx, project, apiBuildOptions)
}
