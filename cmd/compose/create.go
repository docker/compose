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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type createOptions struct {
	Build         bool
	noBuild       bool
	Pull          string
	pullChanged   bool
	removeOrphans bool
	ignoreOrphans bool
	forceRecreate bool
	noRecreate    bool
	recreateDeps  bool
	noInherit     bool
	timeChanged   bool
	timeout       int
	quietPull     bool
	scale         []string
}

func createCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := createOptions{}
	buildOpts := buildOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "create [OPTIONS] [SERVICE...]",
		Short: "Creates containers for a service",
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			opts.pullChanged = cmd.Flags().Changed("pull")
			if opts.Build && opts.noBuild {
				return fmt.Errorf("--build and --no-build are incompatible")
			}
			if opts.forceRecreate && opts.noRecreate {
				return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
			}
			return nil
		}),
		RunE: p.WithServices(dockerCli, func(ctx context.Context, project *types.Project, services []string) error {
			return runCreate(ctx, dockerCli, backend, opts, buildOpts, project, services)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers")
	flags.BoolVar(&opts.noBuild, "no-build", false, "Don't build an image, even if it's policy")
	flags.StringVar(&opts.Pull, "pull", "policy", `Pull image before running ("always"|"missing"|"never"|"build")`)
	flags.BoolVar(&opts.quietPull, "quiet-pull", false, "Pull without printing progress information")
	flags.BoolVar(&opts.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed")
	flags.BoolVar(&opts.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file")
	flags.StringArrayVar(&opts.scale, "scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	return cmd
}

func runCreate(ctx context.Context, _ command.Cli, backend api.Service, createOpts createOptions, buildOpts buildOptions, project *types.Project, services []string) error {
	if err := createOpts.Apply(project); err != nil {
		return err
	}

	var build *api.BuildOptions
	if !createOpts.noBuild {
		bo, err := buildOpts.toAPIBuildOptions(services)
		if err != nil {
			return err
		}
		build = &bo
	}

	return backend.Create(ctx, project, api.CreateOptions{
		Build:                build,
		Services:             services,
		RemoveOrphans:        createOpts.removeOrphans,
		IgnoreOrphans:        createOpts.ignoreOrphans,
		Recreate:             createOpts.recreateStrategy(),
		RecreateDependencies: createOpts.dependenciesRecreateStrategy(),
		Inherit:              !createOpts.noInherit,
		Timeout:              createOpts.GetTimeout(),
		QuietPull:            createOpts.quietPull,
	})
}

func (opts createOptions) recreateStrategy() string {
	if opts.noRecreate {
		return api.RecreateNever
	}
	if opts.forceRecreate {
		return api.RecreateForce
	}
	if opts.noInherit {
		return api.RecreateForce
	}
	return api.RecreateDiverged
}

func (opts createOptions) dependenciesRecreateStrategy() string {
	if opts.noRecreate {
		return api.RecreateNever
	}
	if opts.recreateDeps {
		return api.RecreateForce
	}
	return api.RecreateDiverged
}

func (opts createOptions) GetTimeout() *time.Duration {
	if opts.timeChanged {
		t := time.Duration(opts.timeout) * time.Second
		return &t
	}
	return nil
}

func (opts createOptions) Apply(project *types.Project) error {
	if opts.pullChanged {
		if !opts.isPullPolicyValid() {
			return fmt.Errorf("invalid --pull option %q", opts.Pull)
		}
		for i, service := range project.Services {
			service.PullPolicy = opts.Pull
			project.Services[i] = service
		}
	}
	// N.B. opts.Build means "force build all", but images can still be built
	// when this is false
	// e.g. if a service has pull_policy: build or its local image is policy
	if opts.Build {
		for i, service := range project.Services {
			if service.Build == nil {
				continue
			}
			service.PullPolicy = types.PullPolicyBuild
			project.Services[i] = service
		}
	}

	if err := applyPlatforms(project, true); err != nil {
		return err
	}

	err := applyScaleOpts(project, opts.scale)
	if err != nil {
		return err
	}
	return nil
}

func applyScaleOpts(project *types.Project, opts []string) error {
	for _, scale := range opts {
		split := strings.Split(scale, "=")
		if len(split) != 2 {
			return fmt.Errorf("invalid --scale option %q. Should be SERVICE=NUM", scale)
		}
		name := split[0]
		replicas, err := strconv.Atoi(split[1])
		if err != nil {
			return err
		}
		err = setServiceScale(project, name, replicas)
		if err != nil {
			return err
		}
	}
	return nil
}

func (opts createOptions) isPullPolicyValid() bool {
	pullPolicies := []string{types.PullPolicyAlways, types.PullPolicyNever, types.PullPolicyBuild,
		types.PullPolicyMissing, types.PullPolicyIfNotPresent}
	return slices.Contains(pullPolicies, opts.Pull)
}
