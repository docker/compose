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
	"strings"

	cgo "github.com/compose-spec/compose-go/cli"
	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/cli/cli"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/progress"
)

type runOptions struct {
	*composeOptions
	Service       string
	Command       []string
	environment   []string
	Detach        bool
	Remove        bool
	noTty         bool
	user          string
	workdir       string
	entrypoint    string
	entrypointCmd []string
	labels        []string
	volumes       []string
	publish       []string
	useAliases    bool
	servicePorts  bool
	name          string
	noDeps        bool
}

func (opts runOptions) apply(project *types.Project) error {
	target, err := project.GetService(opts.Service)
	if err != nil {
		return err
	}
	if !opts.servicePorts {
		target.Ports = []types.ServicePortConfig{}
	}
	if len(opts.publish) > 0 {
		target.Ports = []types.ServicePortConfig{}
		for _, p := range opts.publish {
			config, err := types.ParsePortConfig(p)
			if err != nil {
				return err
			}
			target.Ports = append(target.Ports, config...)
		}
	}
	if len(opts.volumes) > 0 {
		for _, v := range opts.volumes {
			volume, err := loader.ParseVolume(v)
			if err != nil {
				return err
			}
			target.Volumes = append(target.Volumes, volume)
		}
	}

	if opts.noDeps {
		for _, s := range project.Services {
			if s.Name != opts.Service {
				project.DisabledServices = append(project.DisabledServices, s)
			}
		}
		project.Services = types.Services{target}
	}

	for i, s := range project.Services {
		if s.Name == opts.Service {
			project.Services[i] = target
			break
		}
	}
	return nil
}

func runCommand(p *projectOptions, backend api.Service) *cobra.Command {
	run := runOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	create := createOptions{}
	cmd := &cobra.Command{
		Use:   "run [options] [-v VOLUME...] [-p PORT...] [-e KEY=VAL...] [-l KEY=VALUE...] SERVICE [COMMAND] [ARGS...]",
		Short: "Run a one-off command on a service.",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			run.Service = args[0]
			if len(args) > 1 {
				run.Command = args[1:]
			}
			if len(run.publish) > 0 && run.servicePorts {
				return fmt.Errorf("--service-ports and --publish are incompatible")
			}
			if cmd.Flags().Changed("entrypoint") {
				command, err := shellwords.Parse(run.entrypoint)
				if err != nil {
					return err
				}
				run.entrypointCmd = command
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			project, err := p.toProject([]string{run.Service}, cgo.WithResolvedPaths(true))
			if err != nil {
				return err
			}
			return runRun(ctx, backend, project, run, create)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&run.Detach, "detach", "d", false, "Run container in background and print container ID")
	flags.StringArrayVarP(&run.environment, "env", "e", []string{}, "Set environment variables")
	flags.StringArrayVarP(&run.labels, "labels", "l", []string{}, "Add or override a label")
	flags.BoolVar(&run.Remove, "rm", false, "Automatically remove the container when it exits")
	flags.BoolVarP(&run.noTty, "no-TTY", "T", notAtTTY(), "Disable pseudo-noTty allocation. By default docker compose run allocates a TTY")
	flags.StringVar(&run.name, "name", "", " Assign a name to the container")
	flags.StringVarP(&run.user, "user", "u", "", "Run as specified username or uid")
	flags.StringVarP(&run.workdir, "workdir", "w", "", "Working directory inside the container")
	flags.StringVar(&run.entrypoint, "entrypoint", "", "Override the entrypoint of the image")
	flags.BoolVar(&run.noDeps, "no-deps", false, "Don't start linked services.")
	flags.StringArrayVarP(&run.volumes, "volume", "v", []string{}, "Bind mount a volume.")
	flags.StringArrayVarP(&run.publish, "publish", "p", []string{}, "Publish a container's port(s) to the host.")
	flags.BoolVar(&run.useAliases, "use-aliases", false, "Use the service's network useAliases in the network(s) the container connects to.")
	flags.BoolVar(&run.servicePorts, "service-ports", false, "Run command with the service's ports enabled and mapped to the host.")

	flags.SetNormalizeFunc(normalizeRunFlags)
	flags.SetInterspersed(false)
	return cmd
}

func normalizeRunFlags(f *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "volumes":
		name = "volume"
	}
	return pflag.NormalizedName(name)
}

func notAtTTY() bool {
	return !isatty.IsTerminal(os.Stdout.Fd())
}

func runRun(ctx context.Context, backend api.Service, project *types.Project, runOptions runOptions, createOptions createOptions) error {
	err := runOptions.apply(project)
	if err != nil {
		return err
	}

	err = progress.Run(ctx, func(ctx context.Context) error {
		return startDependencies(ctx, backend, *project, createOptions, runOptions.Service)
	})
	if err != nil {
		return err
	}

	labels := types.Labels{}
	for _, s := range runOptions.labels {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("label must be set as KEY=VALUE")
		}
		labels[parts[0]] = parts[1]
	}

	// start container and attach to container streams
	runOpts := api.RunOptions{
		Name:              runOptions.name,
		Service:           runOptions.Service,
		Command:           runOptions.Command,
		Detach:            runOptions.Detach,
		AutoRemove:        runOptions.Remove,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               !runOptions.noTty,
		WorkingDir:        runOptions.workdir,
		User:              runOptions.user,
		Environment:       runOptions.environment,
		Entrypoint:        runOptions.entrypointCmd,
		Labels:            labels,
		UseNetworkAliases: runOptions.useAliases,
		Index:             0,
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

func startDependencies(ctx context.Context, backend api.Service, project types.Project, createOptions createOptions, requestedServiceName string) error {
	dependencies := types.Services{}
	var requestedService types.ServiceConfig
	for _, service := range project.Services {
		if service.Name != requestedServiceName {
			dependencies = append(dependencies, service)
		} else {
			requestedService = service
		}
	}

	project.Services = dependencies
	project.DisabledServices = append(project.DisabledServices, requestedService)

	createOptions.Apply(&project)
	create := api.CreateOptions{
		RemoveOrphans:        createOptions.removeOrphans,
		Recreate:             createOptions.recreateStrategy(),
		RecreateDependencies: createOptions.dependenciesRecreateStrategy(),
		Inherit:              !createOptions.noInherit,
		Timeout:              createOptions.GetTimeout(),
		QuietPull:            createOptions.quietPull,
	}

	if err := backend.Create(ctx, &project, create); err != nil {
		return err
	}
	return backend.Start(ctx, &project, api.StartOptions{})
}
