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

package context

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/context/store"
	"github.com/docker/compose-cli/ecs"
	"github.com/docker/compose-cli/errdefs"
)

func init() {
	extraCommands = append(extraCommands, createEcsCommand)
	extraHelp = append(extraHelp, `
Create Amazon ECS context:
$ docker context create ecs CONTEXT [flags]
(see docker context create ecs --help)
`)
}

func createEcsCommand() *cobra.Command {
	var localSimulation bool
	var opts ecs.ContextParams
	cmd := &cobra.Command{
		Use:   "ecs CONTEXT [flags]",
		Short: "Create a context for Amazon ECS",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			if opts.CredsFromEnv && opts.Profile != "" {
				return fmt.Errorf("--profile and --from-env flags cannot be set at the same time")
			}
			if localSimulation {
				return runCreateLocalSimulation(cmd.Context(), args[0], opts)
			}
			return runCreateEcs(cmd.Context(), args[0], opts)
		},
	}

	addDescriptionFlag(cmd, &opts.Description)
	cmd.Flags().BoolVar(&localSimulation, "local-simulation", false, "Create context for ECS local simulation endpoints")
	cmd.Flags().StringVar(&opts.Profile, "profile", "", "Use an existing AWS profile")
	cmd.Flags().BoolVar(&opts.CredsFromEnv, "from-env", false, "Use AWS environment variables for profile, or credentials and region")
	return cmd
}

func runCreateLocalSimulation(ctx context.Context, contextName string, opts ecs.ContextParams) error {
	if contextExists(ctx, contextName) {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "context %q", contextName)
	}
	cs, err := client.GetCloudService(ctx, store.EcsLocalSimulationContextType)
	if err != nil {
		return errors.Wrap(err, "cannot connect to ECS backend")
	}
	data, description, err := cs.CreateContextData(ctx, opts)
	if err != nil {
		return err
	}
	return createDockerContext(ctx, contextName, store.EcsLocalSimulationContextType, description, data)
}

func runCreateEcs(ctx context.Context, contextName string, opts ecs.ContextParams) error {
	if contextExists(ctx, contextName) {
		return errors.Wrapf(errdefs.ErrAlreadyExists, "context %q", contextName)
	}
	contextData, description, err := getEcsContextData(ctx, opts)
	if err != nil {
		return err
	}
	return createDockerContext(ctx, contextName, store.EcsContextType, description, contextData)

}

func getEcsContextData(ctx context.Context, opts ecs.ContextParams) (interface{}, string, error) {
	cs, err := client.GetCloudService(ctx, store.EcsContextType)
	if err != nil {
		return nil, "", errors.Wrap(err, "cannot connect to ECS backend")
	}
	return cs.CreateContextData(ctx, opts)
}
