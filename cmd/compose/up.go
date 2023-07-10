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
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/utils"
)

// composeOptions hold options common to `up` and `run` to run compose project
type composeOptions struct {
	*ProjectOptions
}

type upOptions struct {
	*composeOptions
	Detach             bool
	noStart            bool
	noDeps             bool
	cascadeStop        bool
	exitCodeFrom       string
	noColor            bool
	noPrefix           bool
	attachDependencies bool
	attach             []string
	noAttach           []string
	timestamp          bool
	wait               bool
	waitTimeout        int
}

func (opts upOptions) apply(project *types.Project, services []string) error {
	if opts.noDeps {
		err := project.ForServices(services, types.IgnoreDependencies)
		if err != nil {
			return err
		}
	}

	if opts.exitCodeFrom != "" {
		_, err := project.GetService(opts.exitCodeFrom)
		if err != nil {
			return err
		}
	}

	return nil
}

func upCommand(p *ProjectOptions, streams api.Streams, backend api.Service) *cobra.Command {
	up := upOptions{}
	create := createOptions{}
	upCmd := &cobra.Command{
		Use:   "up [OPTIONS] [SERVICE...]",
		Short: "Create and start containers",
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			create.pullChanged = cmd.Flags().Changed("pull")
			create.timeChanged = cmd.Flags().Changed("timeout")
			return validateFlags(&up, &create)
		}),
		RunE: p.WithServices(func(ctx context.Context, project *types.Project, services []string) error {
			create.ignoreOrphans = utils.StringToBool(project.Environment[ComposeIgnoreOrphans])
			if create.ignoreOrphans && create.removeOrphans {
				return fmt.Errorf("%s and --remove-orphans cannot be combined", ComposeIgnoreOrphans)
			}
			return runUp(ctx, streams, backend, create, up, project, services)
		}),
		ValidArgsFunction: completeServiceNames(p),
	}
	flags := upCmd.Flags()
	flags.BoolVarP(&up.Detach, "detach", "d", false, "Detached mode: Run containers in the background")
	flags.BoolVar(&create.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&create.noBuild, "no-build", false, "Don't build an image, even if it's missing.")
	flags.StringVar(&create.Pull, "pull", "missing", `Pull image before running ("always"|"missing"|"never")`)
	flags.BoolVar(&create.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file.")
	flags.StringArrayVar(&create.scale, "scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	flags.BoolVar(&up.noColor, "no-color", false, "Produce monochrome output.")
	flags.BoolVar(&up.noPrefix, "no-log-prefix", false, "Don't print prefix in logs.")
	flags.BoolVar(&create.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
	flags.BoolVar(&create.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	flags.BoolVar(&up.noStart, "no-start", false, "Don't start the services after creating them.")
	flags.BoolVar(&up.cascadeStop, "abort-on-container-exit", false, "Stops all containers if any container was stopped. Incompatible with -d")
	flags.StringVar(&up.exitCodeFrom, "exit-code-from", "", "Return the exit code of the selected service container. Implies --abort-on-container-exit")
	flags.IntVarP(&create.timeout, "timeout", "t", 0, "Use this timeout in seconds for container shutdown when attached or when containers are already running.")
	flags.BoolVar(&up.timestamp, "timestamps", false, "Show timestamps.")
	flags.BoolVar(&up.noDeps, "no-deps", false, "Don't start linked services.")
	flags.BoolVar(&create.recreateDeps, "always-recreate-deps", false, "Recreate dependent containers. Incompatible with --no-recreate.")
	flags.BoolVarP(&create.noInherit, "renew-anon-volumes", "V", false, "Recreate anonymous volumes instead of retrieving data from the previous containers.")
	flags.BoolVar(&up.attachDependencies, "attach-dependencies", false, "Attach to dependent containers.")
	flags.BoolVar(&create.quietPull, "quiet-pull", false, "Pull without printing progress information.")
	flags.StringArrayVar(&up.attach, "attach", []string{}, "Attach to service output.")
	flags.StringArrayVar(&up.noAttach, "no-attach", []string{}, "Don't attach to specified service.")
	flags.BoolVar(&up.wait, "wait", false, "Wait for services to be running|healthy. Implies detached mode.")
	flags.IntVar(&up.waitTimeout, "wait-timeout", 0, "timeout waiting for application to be running|healthy.")

	return upCmd
}

func validateFlags(up *upOptions, create *createOptions) error {
	if up.exitCodeFrom != "" {
		up.cascadeStop = true
	}
	if up.wait {
		if up.attachDependencies || up.cascadeStop || len(up.attach) > 0 {
			return fmt.Errorf("--wait cannot be combined with --abort-on-container-exit, --attach or --attach-dependencies")
		}
		up.Detach = true
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
}

func runUp(ctx context.Context, streams api.Streams, backend api.Service, createOptions createOptions, upOptions upOptions, project *types.Project, services []string) error {
	if len(project.Services) == 0 {
		return fmt.Errorf("no service selected")
	}

	err := createOptions.Apply(project)
	if err != nil {
		return err
	}

	err = upOptions.apply(project, services)
	if err != nil {
		return err
	}

	var consumer api.LogConsumer
	if !upOptions.Detach {
		consumer = formatter.NewLogConsumer(ctx, streams.Out(), streams.Err(), !upOptions.noColor, !upOptions.noPrefix, upOptions.timestamp)
	}

	attachTo := utils.Set[string]{}
	if len(upOptions.attach) > 0 {
		attachTo.AddAll(upOptions.attach...)
	}
	if upOptions.attachDependencies {
		if err := project.WithServices(attachTo.Elements(), func(s types.ServiceConfig) error {
			if s.Attach == nil || *s.Attach {
				attachTo.Add(s.Name)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if len(attachTo) == 0 {
		if err := project.WithServices(services, func(s types.ServiceConfig) error {
			if s.Attach == nil || *s.Attach {
				attachTo.Add(s.Name)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	attachTo.RemoveAll(upOptions.noAttach...)

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

	timeout := time.Duration(upOptions.waitTimeout) * time.Second

	return backend.Up(ctx, project, api.UpOptions{
		Create: create,
		Start: api.StartOptions{
			Project:      project,
			Attach:       consumer,
			AttachTo:     attachTo.Elements(),
			ExitCodeFrom: upOptions.exitCodeFrom,
			CascadeStop:  upOptions.cascadeStop,
			Wait:         upOptions.wait,
			WaitTimeout:  timeout,
			Services:     services,
		},
	})
}

func setServiceScale(project *types.Project, name string, replicas uint64) error {
	for i, s := range project.Services {
		if s.Name != name {
			continue
		}

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
	return fmt.Errorf("unknown service %q", name)
}
