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
	"strings"

	"github.com/compose-spec/compose-go/v2/format"
	xprogress "github.com/moby/buildkit/util/progress/progressui"
	"github.com/sirupsen/logrus"

	cgo "github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/opts"
	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/docker/compose/v2/pkg/utils"
)

type runOptions struct {
	*composeOptions
	Service       string
	Command       []string
	environment   []string
	Detach        bool
	Remove        bool
	noTty         bool
	tty           bool
	interactive   bool
	user          string
	workdir       string
	entrypoint    string
	entrypointCmd []string
	capAdd        opts.ListOpts
	capDrop       opts.ListOpts
	labels        []string
	volumes       []string
	publish       []string
	useAliases    bool
	servicePorts  bool
	name          string
	noDeps        bool
	ignoreOrphans bool
	removeOrphans bool
	quietPull     bool
}

func (options runOptions) apply(project *types.Project) (*types.Project, error) {
	if options.noDeps {
		var err error
		project, err = project.WithSelectedServices([]string{options.Service}, types.IgnoreDependencies)
		if err != nil {
			return nil, err
		}
	}

	target, err := project.GetService(options.Service)
	if err != nil {
		return nil, err
	}

	target.Tty = !options.noTty
	target.StdinOpen = options.interactive

	// --service-ports and --publish are incompatible
	if !options.servicePorts {
		if len(target.Ports) > 0 {
			logrus.Debug("Running service without ports exposed as --service-ports=false")
		}
		target.Ports = []types.ServicePortConfig{}
		for _, p := range options.publish {
			config, err := types.ParsePortConfig(p)
			if err != nil {
				return nil, err
			}
			target.Ports = append(target.Ports, config...)
		}
	}

	for _, v := range options.volumes {
		volume, err := format.ParseVolume(v)
		if err != nil {
			return nil, err
		}
		target.Volumes = append(target.Volumes, volume)
	}

	for name := range project.Services {
		if name == options.Service {
			project.Services[name] = target
			break
		}
	}
	return project, nil
}

func runCommand(p *ProjectOptions, dockerCli command.Cli, backend api.Service) *cobra.Command {
	options := runOptions{
		composeOptions: &composeOptions{
			ProjectOptions: p,
		},
		capAdd:  opts.NewListOpts(nil),
		capDrop: opts.NewListOpts(nil),
	}
	createOpts := createOptions{}
	buildOpts := buildOptions{
		ProjectOptions: p,
	}
	cmd := &cobra.Command{
		Use:   "run [OPTIONS] SERVICE [COMMAND] [ARGS...]",
		Short: "Run a one-off command on a service",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			options.Service = args[0]
			if len(args) > 1 {
				options.Command = args[1:]
			}
			if len(options.publish) > 0 && options.servicePorts {
				return fmt.Errorf("--service-ports and --publish are incompatible")
			}
			if cmd.Flags().Changed("entrypoint") {
				command, err := shellwords.Parse(options.entrypoint)
				if err != nil {
					return err
				}
				options.entrypointCmd = command
			}
			if cmd.Flags().Changed("tty") {
				if cmd.Flags().Changed("no-TTY") {
					return fmt.Errorf("--tty and --no-TTY can't be used together")
				} else {
					options.noTty = !options.tty
				}
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			project, _, err := p.ToProject(ctx, dockerCli, []string{options.Service}, cgo.WithResolvedPaths(true), cgo.WithDiscardEnvFile)
			if err != nil {
				return err
			}

			if createOpts.quietPull {
				buildOpts.Progress = string(xprogress.QuietMode)
			}

			options.ignoreOrphans = utils.StringToBool(project.Environment[ComposeIgnoreOrphans])
			return runRun(ctx, backend, project, options, createOpts, buildOpts, dockerCli)
		}),
		ValidArgsFunction: completeServiceNames(dockerCli, p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&options.Detach, "detach", "d", false, "Run container in background and print container ID")
	flags.StringArrayVarP(&options.environment, "env", "e", []string{}, "Set environment variables")
	flags.StringArrayVarP(&options.labels, "label", "l", []string{}, "Add or override a label")
	flags.BoolVar(&options.Remove, "rm", false, "Automatically remove the container when it exits")
	flags.BoolVarP(&options.noTty, "no-TTY", "T", !dockerCli.Out().IsTerminal(), "Disable pseudo-TTY allocation (default: auto-detected)")
	flags.StringVar(&options.name, "name", "", "Assign a name to the container")
	flags.StringVarP(&options.user, "user", "u", "", "Run as specified username or uid")
	flags.StringVarP(&options.workdir, "workdir", "w", "", "Working directory inside the container")
	flags.StringVar(&options.entrypoint, "entrypoint", "", "Override the entrypoint of the image")
	flags.Var(&options.capAdd, "cap-add", "Add Linux capabilities")
	flags.Var(&options.capDrop, "cap-drop", "Drop Linux capabilities")
	flags.BoolVar(&options.noDeps, "no-deps", false, "Don't start linked services")
	flags.StringArrayVarP(&options.volumes, "volume", "v", []string{}, "Bind mount a volume")
	flags.StringArrayVarP(&options.publish, "publish", "p", []string{}, "Publish a container's port(s) to the host")
	flags.BoolVar(&options.useAliases, "use-aliases", false, "Use the service's network useAliases in the network(s) the container connects to")
	flags.BoolVarP(&options.servicePorts, "service-ports", "P", false, "Run command with all service's ports enabled and mapped to the host")
	flags.BoolVar(&options.quietPull, "quiet-pull", false, "Pull without printing progress information")
	flags.BoolVar(&createOpts.Build, "build", false, "Build image before starting container")
	flags.BoolVar(&options.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file")

	cmd.Flags().BoolVarP(&options.interactive, "interactive", "i", true, "Keep STDIN open even if not attached")
	cmd.Flags().BoolVarP(&options.tty, "tty", "t", true, "Allocate a pseudo-TTY")
	cmd.Flags().MarkHidden("tty") //nolint:errcheck

	flags.SetNormalizeFunc(normalizeRunFlags)
	flags.SetInterspersed(false)
	return cmd
}

func normalizeRunFlags(f *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "volumes":
		name = "volume"
	case "labels":
		name = "label"
	}
	return pflag.NormalizedName(name)
}

func runRun(ctx context.Context, backend api.Service, project *types.Project, options runOptions, createOpts createOptions, buildOpts buildOptions, dockerCli command.Cli) error {
	project, err := options.apply(project)
	if err != nil {
		return err
	}

	err = createOpts.Apply(project)
	if err != nil {
		return err
	}

	err = progress.Run(ctx, func(ctx context.Context) error {
		var buildForDeps *api.BuildOptions
		if !createOpts.noBuild {
			// allow dependencies needing build to be implicitly selected
			bo, err := buildOpts.toAPIBuildOptions(nil)
			if err != nil {
				return err
			}
			buildForDeps = &bo
		}
		return startDependencies(ctx, backend, *project, buildForDeps, options)
	}, dockerCli.Err())
	if err != nil {
		return err
	}

	labels := types.Labels{}
	for _, s := range options.labels {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("label must be set as KEY=VALUE")
		}
		labels[parts[0]] = parts[1]
	}

	var buildForRun *api.BuildOptions
	if !createOpts.noBuild {
		// dependencies have already been started above, so only the service
		// being run might need to be built at this point
		bo, err := buildOpts.toAPIBuildOptions([]string{options.Service})
		if err != nil {
			return err
		}
		buildForRun = &bo
	}

	// start container and attach to container streams
	runOpts := api.RunOptions{
		Build:             buildForRun,
		Name:              options.name,
		Service:           options.Service,
		Command:           options.Command,
		Detach:            options.Detach,
		AutoRemove:        options.Remove,
		Tty:               !options.noTty,
		Interactive:       options.interactive,
		WorkingDir:        options.workdir,
		User:              options.user,
		CapAdd:            options.capAdd.GetAll(),
		CapDrop:           options.capDrop.GetAll(),
		Environment:       options.environment,
		Entrypoint:        options.entrypointCmd,
		Labels:            labels,
		UseNetworkAliases: options.useAliases,
		NoDeps:            options.noDeps,
		Index:             0,
		QuietPull:         options.quietPull,
	}

	for name, service := range project.Services {
		if name == options.Service {
			service.StdinOpen = options.interactive
			project.Services[name] = service
		}
	}

	exitCode, err := backend.RunOneOffContainer(ctx, project, runOpts)
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
}

func startDependencies(ctx context.Context, backend api.Service, project types.Project, buildOpts *api.BuildOptions, options runOptions) error {
	dependencies := types.Services{}
	var requestedService types.ServiceConfig
	for name, service := range project.Services {
		if name != options.Service {
			dependencies[name] = service
		} else {
			requestedService = service
		}
	}

	project.Services = dependencies
	project.DisabledServices[options.Service] = requestedService
	err := backend.Create(ctx, &project, api.CreateOptions{
		Build:         buildOpts,
		IgnoreOrphans: options.ignoreOrphans,
		RemoveOrphans: options.removeOrphans,
		QuietPull:     options.quietPull,
	})
	if err != nil {
		return err
	}

	if len(dependencies) > 0 {
		return backend.Start(ctx, project.Name, api.StartOptions{
			Project: &project,
		})
	}
	return nil
}
