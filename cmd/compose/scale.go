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
	"strconv"
	"strings"

	"github.com/docker/cli/cli/command"

	"github.com/compose-spec/compose-go/types"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/spf13/cobra"
)

type scaleOptions struct {
	*ProjectOptions
	noDeps bool
}

func scaleCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	opts := scaleOptions{
		ProjectOptions: p,
	}
	scaleCmd := &cobra.Command{
		Use:   "scale [SERVICE=REPLICAS...]",
		Short: "Scale services ",
		Args:  cobra.MinimumNArgs(1),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			serviceTuples, err := parseServicesReplicasArgs(args)
			if err != nil {
				return err
			}
			return runScale(ctx, dockerCli, backend, opts, serviceTuples)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := scaleCmd.Flags()
	flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't start linked services.")

	return scaleCmd
}

func runScale(ctx context.Context, dockerCli command.Cli, backend api.Service, opts scaleOptions, serviceReplicaTuples map[string]int) error {
	services := maps.Keys(serviceReplicaTuples)
	project, err := opts.ToProject(dockerCli, services)
	if err != nil {
		return err
	}

	if opts.noDeps {
		if err := project.ForServices(services, types.IgnoreDependencies); err != nil {
			return err
		}
	}

	for key, value := range serviceReplicaTuples {
		for i, service := range project.Services {
			if service.Name != key {
				continue
			}
			if service.Deploy == nil {
				service.Deploy = &types.DeployConfig{}
			}
			scale := uint64(value)
			service.Deploy.Replicas = &scale
			project.Services[i] = service
			break
		}
	}

	return backend.Scale(ctx, project, api.ScaleOptions{Services: services})
}

func parseServicesReplicasArgs(args []string) (map[string]int, error) {
	serviceReplicaTuples := map[string]int{}
	for _, arg := range args {
		key, val, ok := strings.Cut(arg, "=")
		if !ok || key == "" || val == "" {
			return nil, errors.Errorf("Invalide scale specifier %q.", arg)
		}
		intValue, err := strconv.Atoi(val)

		if err != nil {
			return nil, errors.Errorf("Invalide scale specifier, can't parse replicate value to int %q.", arg)
		}
		serviceReplicaTuples[key] = intValue
	}
	return serviceReplicaTuples, nil
}
