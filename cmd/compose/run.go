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
	opts := runOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	cmd := &cobra.Command{
		Use:   "run [options] [-v VOLUME...] [-p PORT...] [-e KEY=VAL...] [-l KEY=VALUE...] SERVICE [COMMAND] [ARGS...]",
		Short: "Run a one-off command on a service.",
		Args:  cobra.MinimumNArgs(1),
		PreRunE: AdaptCmd(func(ctx context.Context, cmd *cobra.Command, args []string) error {
			opts.Service = args[0]
			if len(args) > 1 {
				opts.Command = args[1:]
			}
			if len(opts.publish) > 0 && opts.servicePorts {
				return fmt.Errorf("--service-ports and --publish are incompatible")
			}
			if cmd.Flags().Changed("entrypoint") {
				command, err := shellwords.Parse(opts.entrypoint)
				if err != nil {
					return err
				}
				opts.entrypointCmd = command
			}
			return nil
		}),
		RunE: Adapt(func(ctx context.Context, args []string) error {
			project, err := p.toProject([]string{opts.Service}, cgo.WithResolvedPaths(true))
			if err != nil {
				return err
			}
			return runRun(ctx, backend, project, opts)
		}),
		ValidArgsFunction: serviceCompletion(p),
	}
	flags := cmd.Flags()
	flags.BoolVarP(&opts.Detach, "detach", "d", false, "Run container in background and print container ID")
	flags.StringArrayVarP(&opts.environment, "env", "e", []string{}, "Set environment variables")
	flags.StringArrayVarP(&opts.labels, "label", "l", []string{}, "Add or override a label")
	flags.BoolVar(&opts.Remove, "rm", false, "Automatically remove the container when it exits")
	flags.BoolVarP(&opts.noTty, "no-TTY", "T", notAtTTY(), "Disable pseudo-noTty allocation. By default docker compose run allocates a TTY")
	flags.StringVar(&opts.name, "name", "", " Assign a name to the container")
	flags.StringVarP(&opts.user, "user", "u", "", "Run as specified username or uid")
	flags.StringVarP(&opts.workdir, "workdir", "w", "", "Working directory inside the container")
	flags.StringVar(&opts.entrypoint, "entrypoint", "", "Override the entrypoint of the image")
	flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't start linked services.")
	flags.StringArrayVarP(&opts.volumes, "volume", "v", []string{}, "Bind mount a volume.")
	flags.StringArrayVarP(&opts.publish, "publish", "p", []string{}, "Publish a container's port(s) to the host.")
	flags.BoolVar(&opts.useAliases, "use-aliases", false, "Use the service's network useAliases in the network(s) the container connects to.")
	flags.BoolVar(&opts.servicePorts, "service-ports", false, "Run command with the service's ports enabled and mapped to the host.")

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

func notAtTTY() bool {
	b := isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd())
	return !b
}

func runRun(ctx context.Context, backend api.Service, project *types.Project, opts runOptions) error {
	err := opts.apply(project)
	if err != nil {
		return err
	}

	err = progress.Run(ctx, func(ctx context.Context) error {
		return startDependencies(ctx, backend, *project, opts.Service)
	})
	if err != nil {
		return err
	}

	labels := types.Labels{}
	for _, s := range opts.labels {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("label must be set as KEY=VALUE")
		}
		labels[parts[0]] = parts[1]
	}

	// start container and attach to container streams
	runOpts := api.RunOptions{
		Name:              opts.name,
		Service:           opts.Service,
		Command:           opts.Command,
		Detach:            opts.Detach,
		AutoRemove:        opts.Remove,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               !opts.noTty,
		WorkingDir:        opts.workdir,
		User:              opts.user,
		Environment:       opts.environment,
		Entrypoint:        opts.entrypointCmd,
		Labels:            labels,
		UseNetworkAliases: opts.useAliases,
		NoDeps:            opts.noDeps,
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

func startDependencies(ctx context.Context, backend api.Service, project types.Project, requestedServiceName string) error {
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
	if err := backend.Create(ctx, &project, api.CreateOptions{}); err != nil {
		return err
	}
	return backend.Start(ctx, &project, api.StartOptions{})
}
