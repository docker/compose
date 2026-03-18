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
	"time"

	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

type deployOptions struct {
	*ProjectOptions
	composeOptions
	build         bool
	noBuild       bool
	push          bool
	quiet         bool
	removeOrphans bool
	wait          bool
	waitTimeout   int
}

func deployCommand(p *ProjectOptions, dockerCli command.Cli, backendOptions *BackendOptions) *cobra.Command {
	opts := deployOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "deploy [OPTIONS] [SERVICE...]",
		Short: "Deploy a Compose application to a Docker server",
		Long: `Deploy a Compose application to a Docker server.

This command applies the Compose project to the target Docker server,
recreating containers with updated configuration and images. Images are
pulled from the registry unless --build is specified.

Use health checks defined in the Compose file to ensure zero-downtime
deployments by passing --wait.`,
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			if opts.waitTimeout < 0 {
				return fmt.Errorf("--wait-timeout must be a non-negative integer")
			}
			if opts.build && opts.noBuild {
				return fmt.Errorf("--build and --no-build are incompatible")
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			return runDeploy(ctx, dockerCli, backendOptions, opts, args)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.build, "build", false, "Build images before deploying")
	flags.BoolVar(&opts.noBuild, "no-build", false, "Do not build images even if build configuration is defined")
	flags.BoolVar(&opts.push, "push", false, "Push images to registry before deploying")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress pull/push progress output")
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file")
	flags.BoolVar(&opts.wait, "wait", false, "Wait for services to be healthy before returning")
	flags.IntVar(&opts.waitTimeout, "wait-timeout", 0, "Maximum duration in seconds to wait for services to be healthy (0 = no timeout)")
	return cmd
}

func runDeploy(ctx context.Context, dockerCli command.Cli, backendOptions *BackendOptions, opts deployOptions, services []string) error {
	backend, err := compose.NewComposeService(dockerCli, backendOptions.Options...)
	if err != nil {
		return err
	}

	project, _, err := opts.ToProject(ctx, dockerCli, backend, services)
	if err != nil {
		return err
	}

	deployOpts := api.DeployOptions{
		Push:          opts.push,
		Quiet:         opts.quiet,
		RemoveOrphans: opts.removeOrphans,
		Wait:          opts.wait,
		Services:      services,
	}

	if opts.waitTimeout > 0 {
		deployOpts.WaitTimeout = time.Duration(opts.waitTimeout) * time.Second
	}

	if opts.build && !opts.noBuild {
		deployOpts.Build = &api.BuildOptions{
			Services: services,
		}
	}

	return backend.Deploy(ctx, project, deployOpts)
}
