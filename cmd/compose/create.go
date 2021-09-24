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

	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
)

type createOptions struct {
	Build         bool
	noBuild       bool
	removeOrphans bool
	ignoreOrphans bool
	forceRecreate bool
	noRecreate    bool
	recreateDeps  bool
	noInherit     bool
	timeChanged   bool
	timeout       int
	quietPull     bool
}

func createCommand(p *projectOptions, backend api.Service) *cobra.Command {
	opts := createOptions{}
	cmd := &cobra.Command{
		Use:   "create [SERVICE...]",
		Short: "Creates containers for a service.",
		PreRunE: Adapt(func(ctx context.Context, args []string) error {
			if opts.Build && opts.noBuild {
				return fmt.Errorf("--build and --no-build are incompatible")
			}
			if opts.forceRecreate && opts.noRecreate {
				return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
			}
			return nil
		}),
		RunE: p.WithProject(func(ctx context.Context, project *types.Project) error {
			return backend.Create(ctx, project, api.CreateOptions{
				RemoveOrphans:        opts.removeOrphans,
				IgnoreOrphans:        opts.ignoreOrphans,
				Recreate:             opts.recreateStrategy(),
				RecreateDependencies: opts.dependenciesRecreateStrategy(),
				Inherit:              !opts.noInherit,
				Timeout:              opts.GetTimeout(),
				QuietPull:            false,
			})
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&opts.noBuild, "no-build", false, "Don't build an image, even if it's missing.")
	flags.BoolVar(&opts.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	flags.BoolVar(&opts.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	return cmd
}

func (opts createOptions) recreateStrategy() string {
	if opts.noRecreate {
		return api.RecreateNever
	}
	if opts.forceRecreate {
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

func (opts createOptions) Apply(project *types.Project) {
	if opts.Build {
		for i, service := range project.Services {
			if service.Build == nil {
				continue
			}
			service.PullPolicy = types.PullPolicyBuild
			project.Services[i] = service
		}
	}
	if opts.noBuild {
		for i, service := range project.Services {
			service.Build = nil
			project.Services[i] = service
		}
	}
}
