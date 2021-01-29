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
	"errors"
	"fmt"
	"os"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/cli/formatter"

	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"
)

// composeOptions hold options common to `up` and `run` to run compose project
type composeOptions struct {
	*projectOptions
	Build bool
	// ACI only
	DomainName string
}

type upOptions struct {
	*composeOptions
	Detach        bool
	Environment   []string
	removeOrphans bool
	forceRecreate bool
	noRecreate    bool
}

func (o upOptions) recreateStrategy() string {
	if o.noRecreate {
		return compose.RecreateNever
	}
	if o.forceRecreate {
		return compose.RecreateForce
	}
	return compose.RecreateDiverged
}

func upCommand(p *projectOptions, contextType string) *cobra.Command {
	opts := upOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	upCmd := &cobra.Command{
		Use:   "up [SERVICE...]",
		Short: "Create and start containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch contextType {
			case store.LocalContextType, store.DefaultContextType, store.EcsLocalSimulationContextType:
				if opts.forceRecreate && opts.noRecreate {
					return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
				}
				return runCreateStart(cmd.Context(), opts, args)
			default:
				return runUp(cmd.Context(), opts, args)
			}
		},
	}
	flags := upCmd.Flags()
	flags.StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")
	flags.BoolVarP(&opts.Detach, "detach", "d", false, "Detached mode: Run containers in the background")
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file.")

	switch contextType {
	case store.AciContextType:
		flags.StringVar(&opts.DomainName, "domainname", "", "Container NIS domain name")
	case store.LocalContextType, store.DefaultContextType, store.EcsLocalSimulationContextType:
		flags.BoolVar(&opts.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
		flags.BoolVar(&opts.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")

	}

	return upCmd
}

func runUp(ctx context.Context, opts upOptions, services []string) error {
	c, project, err := setup(ctx, *opts.composeOptions, services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Up(ctx, project, compose.UpOptions{
			Detach: opts.Detach,
		})
	})
	return err
}

func runCreateStart(ctx context.Context, opts upOptions, services []string) error {
	c, project, err := setup(ctx, *opts.composeOptions, services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Create(ctx, project, compose.CreateOptions{
			RemoveOrphans: opts.removeOrphans,
			Recreate:      opts.recreateStrategy(),
		})
	})
	if err != nil {
		return err
	}

	var consumer compose.LogConsumer
	if !opts.Detach {
		consumer = formatter.NewLogConsumer(ctx, os.Stdout)
	}

	err = c.ComposeService().Start(ctx, project, consumer)
	if errors.Is(ctx.Err(), context.Canceled) {
		fmt.Println("Gracefully stopping...")
		ctx = context.Background()
		_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
			return "", c.ComposeService().Stop(ctx, project, consumer)
		})
	}
	return err
}

func setup(ctx context.Context, opts composeOptions, services []string) (*client.Client, *types.Project, error) {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return nil, nil, err
	}

	project, err := opts.toProject()
	if err != nil {
		return nil, nil, err
	}
	if opts.DomainName != "" {
		// arbitrarily set the domain name on the first service ; ACI backend will expose the entire project
		project.Services[0].DomainName = opts.DomainName
	}
	if opts.Build {
		for _, service := range project.Services {
			service.PullPolicy = types.PullPolicyBuild
		}
	}

	err = filter(project, services)
	if err != nil {
		return nil, nil, err
	}
	return c, project, nil
}
