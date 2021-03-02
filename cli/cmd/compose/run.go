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

	"github.com/compose-spec/compose-go/types"
	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/cli/cmd"
)

type runOptions struct {
	*composeOptions
	Service     string
	Command     []string
	environment []string
	Detach      bool
	Remove      bool
	noTty       bool
	user        string
	workdir     string
	entrypoint  string
	labels      []string
	name        string
	noDeps      bool
}

func runCommand(p *projectOptions) *cobra.Command {
	opts := runOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	cmd := &cobra.Command{
		Use:   "run [options] [-v VOLUME...] [-p PORT...] [-e KEY=VAL...] [-l KEY=VALUE...] SERVICE [COMMAND] [ARGS...]",
		Short: "Run a one-off command on a service.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				opts.Command = args[1:]
			}
			opts.Service = args[0]
			return runRun(cmd.Context(), opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVarP(&opts.Detach, "detach", "d", false, "Run container in background and print container ID")
	flags.StringArrayVarP(&opts.environment, "env", "e", []string{}, "Set environment variables")
	flags.StringArrayVarP(&opts.labels, "labels", "l", []string{}, "Add or override a label")
	flags.BoolVar(&opts.Remove, "rm", false, "Automatically remove the container when it exits")
	flags.BoolVarP(&opts.noTty, "no-TTY", "T", false, "Disable pseudo-tty allocation. By default docker compose run allocates a TTY")
	flags.StringVar(&opts.name, "name", "", " Assign a name to the container")
	flags.StringVarP(&opts.user, "user", "u", "", "Run as specified username or uid")
	flags.StringVarP(&opts.workdir, "workdir", "w", "", "Working directory inside the container")
	flags.StringVar(&opts.entrypoint, "entrypoint", "", "Override the entrypoint of the image")
	flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't start linked services.")

	flags.SetInterspersed(false)
	return cmd
}

func runRun(ctx context.Context, opts runOptions) error {
	c, project, err := setup(ctx, *opts.composeOptions, []string{opts.Service})
	if err != nil {
		return err
	}

	if opts.noDeps {
		enabled, err := project.GetService(opts.Service)
		if err != nil {
			return err
		}
		for _, s := range project.Services {
			if s.Name != opts.Service {
				project.DisabledServices = append(project.DisabledServices, s)
			}
		}
		project.Services = types.Services{enabled}
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", startDependencies(ctx, c, *project, opts.Service)
	})
	if err != nil {
		return err
	}

	var entrypoint []string
	if opts.entrypoint != "" {
		entrypoint, err = shellwords.Parse(opts.entrypoint)
		if err != nil {
			return err
		}
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
	runOpts := compose.RunOptions{
		Name:        opts.name,
		Service:     opts.Service,
		Command:     opts.Command,
		Detach:      opts.Detach,
		AutoRemove:  opts.Remove,
		Writer:      os.Stdout,
		Reader:      os.Stdin,
		Tty:         !opts.noTty,
		WorkingDir:  opts.workdir,
		User:        opts.user,
		Environment: opts.environment,
		Entrypoint:  entrypoint,
		Labels:      labels,
		Index:       0,
	}
	exitCode, err := c.ComposeService().RunOneOffContainer(ctx, project, runOpts)
	if exitCode != 0 {
		return cmd.ExitCodeError{ExitCode: exitCode}
	}
	return err
}

func startDependencies(ctx context.Context, c *client.Client, project types.Project, requestedServiceName string) error {
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
	if err := c.ComposeService().Create(ctx, &project, compose.CreateOptions{}); err != nil {
		return err
	}
	if err := c.ComposeService().Start(ctx, &project, compose.StartOptions{}); err != nil {
		return err
	}
	return nil
}
