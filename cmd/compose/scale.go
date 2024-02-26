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
	"strconv"
	"strings"

	"github.com/docker/cli/cli/command"

	"github.com/compose-spec/compose-go/v2/types"
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
	flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't start linked services")

	return scaleCmd
}

func runScale(ctx context.Context, dockerCli command.Cli, backend api.Service, opts scaleOptions, serviceReplicaTuples map[string]int) error {
	services := maps.Keys(serviceReplicaTuples)
	project, _, err := opts.ToProject(ctx, dockerCli, services)
	if err != nil {
		return err
	}

	if opts.noDeps {
		if project, err = project.WithSelectedServices(services, types.IgnoreDependencies); err != nil {
			return err
		}
	}

	for key, value := range serviceReplicaTuples {
		service, err := project.GetService(key)
		if err != nil {
			return err
		}
		service.SetScale(value)
		project.Services[key] = service
	}

	return backend.Scale(ctx, project, api.ScaleOptions{Services: services})
}

func parseServicesReplicasArgs(args []string) (map[string]int, error) {
	serviceReplicaTuples := map[string]int{}
	for _, arg := range args {
		key, val, ok := strings.Cut(arg, "=")
		if !ok || key == "" || val == "" {
			return nil, fmt.Errorf("invalid scale specifier: %s", arg)
		}
		intValue, err := strconv.Atoi(val)

		if err != nil {
			return nil, fmt.Errorf("invalid scale specifier: can't parse replica value as int: %v", arg)
		}
		serviceReplicaTuples[key] = intValue
	}
	return serviceReplicaTuples, nil
}
