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
	"strconv"
	"strings"

	"github.com/docker/compose/v2/cmd/formatter"

	"github.com/compose-spec/compose-go/types"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

// composeOptions hold options common to `up` and `run` to run compose project
type composeOptions struct {
	*projectOptions
}

type upOptions struct {
	*composeOptions
	Detach             bool
	Environment        []string
	noStart            bool
	noDeps             bool
	cascadeStop        bool
	exitCodeFrom       string
	scale              []string
	noColor            bool
	noPrefix           bool
	attachDependencies bool
	attach             []string
}

func (opts upOptions) apply(project *types.Project, services []string) error {
	if opts.noDeps {
		enabled, err := project.GetServices(services...)
		if err != nil {
			return err
		}
		for _, s := range project.Services {
			if !utils.StringContains(services, s.Name) {
				project.DisabledServices = append(project.DisabledServices, s)
			}
		}
		project.Services = enabled
	}

	if opts.exitCodeFrom != "" {
		_, err := project.GetService(opts.exitCodeFrom)
		if err != nil {
			return err
		}
	}

	for _, scale := range opts.scale {
		split := strings.Split(scale, "=")
		if len(split) != 2 {
			return fmt.Errorf("invalid --scale option %q. Should be SERVICE=NUM", scale)
		}
		name := split[0]
		replicas, err := strconv.Atoi(split[1])
		if err != nil {
			return err
		}
		err = setServiceScale(project, name, uint64(replicas))
		if err != nil {
			return err
		}
	}

	return nil
}

func upCommand(p *projectOptions, backend api.Service) *cobra.Command {
	up := upOptions{}
	create := createOptions{}
	upCmd := &cobra.Command{
		Use:   "up [SERVICE...]",
		Short: "Create and start containers",
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			create.timeChanged = cmd.Flags().Changed("timeout")
			if up.exitCodeFrom != "" {
				up.cascadeStop = true
			}
			if create.Build && create.noBuild {
				return fmt.Errorf("--build and --no-build are incompatible")
			}
			if up.Detach && (up.attachDependencies || up.cascadeStop || len(up.attach) > 0) {
				return fmt.Errorf("--detach cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies")
			}
			if create.forceRecreate && create.noRecreate {
				return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
			}
			if create.recreateDeps && create.noRecreate {
				return fmt.Errorf("--always-recreate-deps and --no-recreate are incompatible")
			}
			return nil
		}),
		RunE: p.WithServices(func(ctx context.Context, project *types.Project, services []string) error {
			ignore := project.Environment["COMPOSE_IGNORE_ORPHANS"]
			create.ignoreOrphans = strings.ToLower(ignore) == "true"
			if create.ignoreOrphans && create.removeOrphans {
				return fmt.Errorf("COMPOSE_IGNORE_ORPHANS and --remove-orphans cannot be combined")
			}
			return runUp(ctx, backend, create, up, project, services)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := upCmd.Flags()
	flags.StringArrayVarP(&up.Environment, "environment", "e", []string{}, "Environment variables")
	flags.BoolVarP(&up.Detach, "detach", "d", false, "Detached mode: Run containers in the background")
	flags.BoolVar(&create.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&create.noBuild, "no-build", false, "Don't build an image, even if it's missing.")
	flags.BoolVar(&create.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file.")
	flags.StringArrayVar(&up.scale, "scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	flags.BoolVar(&up.noColor, "no-color", false, "Produce monochrome output.")
	flags.BoolVar(&up.noPrefix, "no-log-prefix", false, "Don't print prefix in logs.")
	flags.BoolVar(&create.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	flags.BoolVar(&create.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	flags.BoolVar(&up.noStart, "no-start", false, "Don't start the services after creating them.")
	flags.BoolVar(&up.cascadeStop, "abort-on-container-exit", false, "Stops all containers if any container was stopped. Incompatible with -d")
	flags.StringVar(&up.exitCodeFrom, "exit-code-from", "", "Return the exit code of the selected service container. Implies --abort-on-container-exit")
	flags.IntVarP(&create.timeout, "timeout", "t", 10, "Use this timeout in seconds for container shutdown when attached or when containers are already running.")
	flags.BoolVar(&up.noDeps, "no-deps", false, "Don't start linked services.")
	flags.BoolVar(&create.recreateDeps, "always-recreate-deps", false, "Recreate dependent containers. Incompatible with --no-recreate.")
	flags.BoolVarP(&create.noInherit, "renew-anon-volumes", "V", false, "Recreate anonymous volumes instead of retrieving data from the previous containers.")
	flags.BoolVar(&up.attachDependencies, "attach-dependencies", false, "Attach to dependent containers.")
	flags.BoolVar(&create.quietPull, "quiet-pull", false, "Pull without printing progress information.")
	flags.StringArrayVar(&up.attach, "attach", []string{}, "Attach to service output.")

	return upCmd
}

func runUp(ctx context.Context, backend api.Service, createOptions createOptions, upOptions upOptions, project *types.Project, services []string) error {
	if len(project.Services) == 0 {
		return fmt.Errorf("no service selected")
	}

	createOptions.Apply(project)

	err := upOptions.apply(project, services)
	if err != nil {
		return err
	}

	var consumer api.LogConsumer
	if !upOptions.Detach {
		consumer = formatter.NewLogConsumer(ctx, os.Stdout, !upOptions.noColor, !upOptions.noPrefix)
	}

	attachTo := services
	if len(upOptions.attach) > 0 {
		attachTo = upOptions.attach
	}
	if upOptions.attachDependencies {
		attachTo = project.ServiceNames()
	}

	create := api.CreateOptions{
		Services:             services,
		RemoveOrphans:        createOptions.removeOrphans,
		IgnoreOrphans:        createOptions.ignoreOrphans,
		Recreate:             createOptions.recreateStrategy(),
		RecreateDependencies: createOptions.dependenciesRecreateStrategy(),
		Inherit:              !createOptions.noInherit,
		Timeout:              createOptions.GetTimeout(),
		QuietPull:            createOptions.quietPull,
	}

	if upOptions.noStart {
		return backend.Create(ctx, project, create)
	}

	return backend.Up(ctx, project, api.UpOptions{
		Create: create,
		Start: api.StartOptions{
			Attach:       consumer,
			AttachTo:     attachTo,
			ExitCodeFrom: upOptions.exitCodeFrom,
			CascadeStop:  upOptions.cascadeStop,
		},
	})
}

func setServiceScale(project *types.Project, name string, replicas uint64) error {
	for i, s := range project.Services {
		if s.Name == name {
			service, err := project.GetService(name)
			if err != nil {
				return err
			}
			if service.Deploy == nil {
				service.Deploy = &types.DeployConfig{}
			}
			service.Deploy.Replicas = &replicas
			project.Services[i] = service
			return nil
		}
	}
	return fmt.Errorf("unknown service %q", name)
}
