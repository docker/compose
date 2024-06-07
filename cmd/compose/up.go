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
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/compose/v2/cmd/formatter"
	"github.com/docker/compose/v2/internal/experimental"
	xprogress "github.com/moby/buildkit/util/progress/progressui"
	"github.com/spf13/cobra"

	"github.com/docker/compose/v2/pkg/api"
	ui "github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

// composeOptions hold options common to `up` and `run` to run compose project
type composeOptions struct {
	*ProjectOptions
}

type upOptions struct {
	*composeOptions
	Detach                bool
	noStart               bool
	noDeps                bool
	cascadeStop           bool
	cascadeFail           bool
	exitCodeFrom          string
	noColor               bool
	noPrefix              bool
	attachDependencies    bool
	attach                []string
	noAttach              []string
	timestamp             bool
	wait                  bool
	waitTimeout           int
	watch                 bool
	navigationMenu        bool
	navigationMenuChanged bool
}

func (opts upOptions) apply(project *types.Project, services []string) (*types.Project, error) {
	if opts.noDeps {
		var err error
		project, err = project.WithSelectedServices(services, types.IgnoreDependencies)
		if err != nil {
			return nil, err
		}
	}

	if opts.exitCodeFrom != "" {
		_, err := project.GetService(opts.exitCodeFrom)
		if err != nil {
			return nil, err
		}
	}

	return project, nil
}

func (opts *upOptions) validateNavigationMenu(dockerCli command.Cli, experimentals *experimental.State) {
	if !dockerCli.Out().IsTerminal() {
		opts.navigationMenu = false
		return
	}
	if !opts.navigationMenuChanged {
		opts.navigationMenu = SetUnchangedOption(ComposeMenu, experimentals.NavBar())
	}
}

func (opts upOptions) OnExit() api.Cascade {
	switch {
	case opts.cascadeStop:
		return api.CascadeStop
	case opts.cascadeFail:
		return api.CascadeFail
	default:
		return api.CascadeIgnore
	}
}

func upCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service, experiments *experimental.State) *cobra.Command {
	up := upOptions{}
	create := createOptions{}
	build := buildOptions{ProjectOptions: p}
	upCmd := &cobra.Command{
		Use:   "up [OPTIONS] [SERVICE...]",
		Short: "Create and start containers",
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			create.pullChanged = cmd.Flags().Changed("pull")
			create.timeChanged = cmd.Flags().Changed("timeout")
			up.navigationMenuChanged = cmd.Flags().Changed("menu")
			if !cmd.Flags().Changed("remove-orphans") {
				create.removeOrphans = utils.StringToBool(os.Getenv(ComposeRemoveOrphans))
			}
			return validateFlags(&up, &create)
		}),
		RunE: p.WithServices(dockerCli, func(ctx context.Context, project *types.Project, services []string) error {
			create.ignoreOrphans = utils.StringToBool(project.Environment[ComposeIgnoreOrphans])
			if create.ignoreOrphans && create.removeOrphans {
				return fmt.Errorf("cannot combine %s and --remove-orphans", ComposeIgnoreOrphans)
			}
			if len(up.attach) != 0 && up.attachDependencies {
				return errors.New("cannot combine --attach and --attach-dependencies")
			}

			up.validateNavigationMenu(dockerCli, experiments)

			if !p.All && len(project.Services) == 0 {
				return fmt.Errorf("no service selected")
			}

			return runUp(ctx, dockerCli, backend, create, up, build, project, services)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := upCmd.Flags()
	flags.BoolVarP(&up.Detach, "detach", "d", false, "Detached mode: Run containers in the background")
	flags.BoolVar(&create.Build, "build", false, "Build images before starting containers")
	flags.BoolVar(&create.noBuild, "no-build", false, "Don't build an image, even if it's policy")
	flags.StringVar(&create.Pull, "pull", "policy", `Pull image before running ("always"|"missing"|"never")`)
	flags.BoolVar(&create.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file")
	flags.StringArrayVar(&create.scale, "scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	flags.BoolVar(&up.noColor, "no-color", false, "Produce monochrome output")
	flags.BoolVar(&up.noPrefix, "no-log-prefix", false, "Don't print prefix in logs")
	flags.BoolVar(&create.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed")
	flags.BoolVar(&create.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
	flags.BoolVar(&up.noStart, "no-start", false, "Don't start the services after creating them")
	flags.BoolVar(&up.cascadeStop, "abort-on-container-exit", false, "Stops all containers if any container was stopped. Incompatible with -d")
	flags.BoolVar(&up.cascadeFail, "abort-on-container-failure", false, "Stops all containers if any container exited with failure. Incompatible with -d")
	flags.StringVar(&up.exitCodeFrom, "exit-code-from", "", "Return the exit code of the selected service container. Implies --abort-on-container-exit")
	flags.IntVarP(&create.timeout, "timeout", "t", 0, "Use this timeout in seconds for container shutdown when attached or when containers are already running")
	flags.BoolVar(&up.timestamp, "timestamps", false, "Show timestamps")
	flags.BoolVar(&up.noDeps, "no-deps", false, "Don't start linked services")
	flags.BoolVar(&create.recreateDeps, "always-recreate-deps", false, "Recreate dependent containers. Incompatible with --no-recreate.")
	flags.BoolVarP(&create.noInherit, "renew-anon-volumes", "V", false, "Recreate anonymous volumes instead of retrieving data from the previous containers")
	flags.BoolVar(&create.quietPull, "quiet-pull", false, "Pull without printing progress information")
	flags.StringArrayVar(&up.attach, "attach", []string{}, "Restrict attaching to the specified services. Incompatible with --attach-dependencies.")
	flags.StringArrayVar(&up.noAttach, "no-attach", []string{}, "Do not attach (stream logs) to the specified services")
	flags.BoolVar(&up.attachDependencies, "attach-dependencies", false, "Automatically attach to log output of dependent services")
	flags.BoolVar(&up.wait, "wait", false, "Wait for services to be running|healthy. Implies detached mode.")
	flags.IntVar(&up.waitTimeout, "wait-timeout", 0, "Maximum duration to wait for the project to be running|healthy")
	flags.BoolVarP(&up.watch, "watch", "w", false, "Watch source code and rebuild/refresh containers when files are updated.")
	flags.BoolVar(&up.navigationMenu, "menu", false, "Enable interactive shortcuts when running attached. Incompatible with --detach. Can also be enable/disable by setting COMPOSE_MENU environment var.")

	return upCmd
}

//nolint:gocyclo
func validateFlags(up *upOptions, create *createOptions) error {
	if up.exitCodeFrom != "" && !up.cascadeFail {
		up.cascadeStop = true
	}
	if up.cascadeStop && up.cascadeFail {
		return fmt.Errorf("--abort-on-container-failure cannot be combined with --abort-on-container-exit")
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
	if up.Detach && (up.attachDependencies || up.cascadeStop || up.cascadeFail || len(up.attach) > 0 || up.watch) {
		return fmt.Errorf("--detach cannot be combined with --abort-on-container-exit, --abort-on-container-failure, --attach, --attach-dependencies or --watch")
	}
	if create.forceRecreate && create.noRecreate {
		return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
	}
	if create.recreateDeps && create.noRecreate {
		return fmt.Errorf("--always-recreate-deps and --no-recreate are incompatible")
	}
	if create.noBuild && up.watch {
		return fmt.Errorf("--no-build and --watch are incompatible")
	}
	return nil
}

func runUp(
	ctx context.Context,
	dockerCli command.Cli,
	backend api.Service,
	createOptions createOptions,
	upOptions upOptions,
	buildOptions buildOptions,
	project *types.Project,
	services []string,
) error {
	err := createOptions.Apply(project)
	if err != nil {
		return err
	}

	project, err = upOptions.apply(project, services)
	if err != nil {
		return err
	}

	var build *api.BuildOptions
	if !createOptions.noBuild {
		if createOptions.quietPull {
			buildOptions.Progress = string(xprogress.QuietMode)
		}
		// BuildOptions here is nested inside CreateOptions, so
		// no service list is passed, it will implicitly pick all
		// services being created, which includes any explicitly
		// specified via "services" arg here as well as deps
		bo, err := buildOptions.toAPIBuildOptions(nil)
		if err != nil {
			return err
		}
		build = &bo
	}

	create := api.CreateOptions{
		Build:                build,
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

	var consumer api.LogConsumer
	var attach []string
	if !upOptions.Detach {
		consumer = formatter.NewLogConsumer(ctx, dockerCli.Out(), dockerCli.Err(), !upOptions.noColor, !upOptions.noPrefix, upOptions.timestamp)

		var attachSet utils.Set[string]
		if len(upOptions.attach) != 0 {
			// services are passed explicitly with --attach, verify they're valid and then use them as-is
			attachSet = utils.NewSet(upOptions.attach...)
			unexpectedSvcs := attachSet.Diff(utils.NewSet(project.ServiceNames()...))
			if len(unexpectedSvcs) != 0 {
				return fmt.Errorf("cannot attach to services not included in up: %s", strings.Join(unexpectedSvcs.Elements(), ", "))
			}
		} else {
			// mark services being launched (and potentially their deps) for attach
			// if they didn't opt-out via Compose YAML
			attachSet = utils.NewSet[string]()
			var dependencyOpt types.DependencyOption = types.IgnoreDependencies
			if upOptions.attachDependencies {
				dependencyOpt = types.IncludeDependencies
			}
			if err := project.ForEachService(services, func(serviceName string, s *types.ServiceConfig) error {
				if s.Attach == nil || *s.Attach {
					attachSet.Add(serviceName)
				}
				return nil
			}, dependencyOpt); err != nil {
				return err
			}
		}
		// filter out any services that have been explicitly marked for ignore with `--no-attach`
		attachSet.RemoveAll(upOptions.noAttach...)
		attach = attachSet.Elements()
	}

	timeout := time.Duration(upOptions.waitTimeout) * time.Second
	return backend.Up(ctx, project, api.UpOptions{
		Create: create,
		Start: api.StartOptions{
			Project:        project,
			Attach:         consumer,
			AttachTo:       attach,
			ExitCodeFrom:   upOptions.exitCodeFrom,
			OnExit:         upOptions.OnExit(),
			Wait:           upOptions.wait,
			WaitTimeout:    timeout,
			Watch:          upOptions.watch,
			Services:       services,
			NavigationMenu: upOptions.navigationMenu && ui.Mode != "plain",
		},
	})
}

func setServiceScale(project *types.Project, name string, replicas int) error {
	service, err := project.GetService(name)
	if err != nil {
		return err
	}
	service.SetScale(replicas)
	project.Services[name] = service
	return nil
}
