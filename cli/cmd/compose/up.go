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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/compose-cli/api/client"
	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/cli/cmd"
	"github.com/docker/compose-cli/cli/formatter"

	"github.com/compose-spec/compose-go/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// composeOptions hold options common to `up` and `run` to run compose project
type composeOptions struct {
	*projectOptions
	Build   bool
	noBuild bool
	// ACI only
	DomainName string
}

type upOptions struct {
	*composeOptions
	Detach        bool
	Environment   []string
	removeOrphans bool
	forceRecreate bool
	noRecreate    bool
	noStart       bool
	cascadeStop   bool
	exitCodeFrom  string
	scale         []string
	noColor       bool
	noPrefix      bool
	timeChanged   bool
	timeout       int
	noDeps        bool
}

func (o upOptions) recreateStrategy() string {
	if o.noRecreate {
		return compose.RecreateNever
	}
	if o.forceRecreate {
		return compose.RecreateForce
	}
	return compose.RecreateDiverged
}

func upCommand(p *projectOptions, contextType string) *cobra.Command {
	opts := upOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	upCmd := &cobra.Command{
		Use:   "up [SERVICE...]",
		Short: "Create and start containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.timeChanged = cmd.Flags().Changed("timeout")
			switch contextType {
			case store.LocalContextType, store.DefaultContextType, store.EcsLocalSimulationContextType:
				if opts.exitCodeFrom != "" {
					opts.cascadeStop = true
				}
				if opts.Build && opts.noBuild {
					return fmt.Errorf("--build and --no-build are incompatible")
				}
				if opts.cascadeStop && opts.Detach {
					return fmt.Errorf("--abort-on-container-exit and --detach are incompatible")
				}
				if opts.forceRecreate && opts.noRecreate {
					return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
				}
				return runCreateStart(cmd.Context(), opts, args)
			default:
				return runUp(cmd.Context(), opts, args)
			}
		},
	}
	flags := upCmd.Flags()
	flags.StringArrayVarP(&opts.Environment, "environment", "e", []string{}, "Environment variables")
	flags.BoolVarP(&opts.Detach, "detach", "d", false, "Detached mode: Run containers in the background")
	flags.BoolVar(&opts.Build, "build", false, "Build images before starting containers.")
	flags.BoolVar(&opts.noBuild, "no-build", false, "Don't build an image, even if it's missing.")
	flags.BoolVar(&opts.removeOrphans, "remove-orphans", false, "Remove containers for services not defined in the Compose file.")
	flags.StringArrayVar(&opts.scale, "scale", []string{}, "Scale SERVICE to NUM instances. Overrides the `scale` setting in the Compose file if present.")
	flags.BoolVar(&opts.noColor, "no-color", false, "Produce monochrome output.")
	flags.BoolVar(&opts.noPrefix, "no-log-prefix", false, "Don't print prefix in logs.")

	switch contextType {
	case store.AciContextType:
		flags.StringVar(&opts.DomainName, "domainname", "", "Container NIS domain name")
	case store.LocalContextType, store.DefaultContextType, store.EcsLocalSimulationContextType:
		flags.BoolVar(&opts.forceRecreate, "force-recreate", false, "Recreate containers even if their configuration and image haven't changed.")
		flags.BoolVar(&opts.noRecreate, "no-recreate", false, "If containers already exist, don't recreate them. Incompatible with --force-recreate.")
		flags.BoolVar(&opts.noStart, "no-start", false, "Don't start the services after creating them.")
		flags.BoolVar(&opts.cascadeStop, "abort-on-container-exit", false, "Stops all containers if any container was stopped. Incompatible with -d")
		flags.StringVar(&opts.exitCodeFrom, "exit-code-from", "", "Return the exit code of the selected service container. Implies --abort-on-container-exit")
		flags.IntVarP(&opts.timeout, "timeout", "t", 10, "Use this timeout in seconds for container shutdown when attached or when containers are already running.")
		flags.BoolVar(&opts.noDeps, "no-deps", false, "Don't start linked services.")
	}

	return upCmd
}

func runUp(ctx context.Context, opts upOptions, services []string) error {
	c, project, err := setup(ctx, *opts.composeOptions, services)
	if err != nil {
		return err
	}

	err = applyScaleOpt(opts.scale, project)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", c.ComposeService().Up(ctx, project, compose.UpOptions{
			Detach: opts.Detach,
		})
	})
	return err
}

func runCreateStart(ctx context.Context, opts upOptions, services []string) error {
	c, project, err := setup(ctx, *opts.composeOptions, services)
	if err != nil {
		return err
	}

	if opts.noDeps {
		enabled, err := project.GetServices(services)
		if err != nil {
			return err
		}
		project.DisabledServices = append(project.DisabledServices, project.Services...)
		project.Services = enabled
	}

	err = applyScaleOpt(opts.scale, project)
	if err != nil {
		return err
	}

	if opts.exitCodeFrom != "" {
		_, err := project.GetService(opts.exitCodeFrom)
		if err != nil {
			return err
		}
	}

	if opts.timeChanged {
		timeoutValue := types.Duration(time.Duration(opts.timeout) * time.Second)
		for i, s := range project.Services {
			s.StopGracePeriod = &timeoutValue
			project.Services[i] = s
		}
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		err := c.ComposeService().Create(ctx, project, compose.CreateOptions{
			RemoveOrphans: opts.removeOrphans,
			Recreate:      opts.recreateStrategy(),
		})
		if err != nil {
			return "", err
		}
		if opts.Detach {
			err = c.ComposeService().Start(ctx, project, compose.StartOptions{})
		}
		return "", err
	})
	if err != nil {
		return err
	}

	if opts.noStart {
		return nil
	}

	if opts.Detach {
		return nil
	}

	queue := make(chan compose.ContainerEvent)
	printer := printer{
		queue: queue,
	}

	stopFunc := func() error {
		ctx := context.Background()
		_, err := progress.Run(ctx, func(ctx context.Context) (string, error) {
			return "", c.ComposeService().Stop(ctx, project, compose.StopOptions{})
		})
		return err
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		queue <- compose.ContainerEvent{
			Type: compose.UserCancel,
		}
		fmt.Println("Gracefully stopping...")
		stopFunc() // nolint:errcheck
	}()

	consumer := formatter.NewLogConsumer(ctx, os.Stdout, !opts.noColor, !opts.noPrefix)

	var exitCode int
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		code, err := printer.run(ctx, opts.cascadeStop, opts.exitCodeFrom, consumer, stopFunc)
		exitCode = code
		return err
	})

	err = c.ComposeService().Start(ctx, project, compose.StartOptions{
		Attach: func(event compose.ContainerEvent) {
			queue <- event
		},
	})
	if err != nil {
		return err
	}

	err = eg.Wait()
	if exitCode != 0 {
		return cmd.ExitCodeError{ExitCode: exitCode}
	}
	return err
}

func applyScaleOpt(opts []string, project *types.Project) error {
	for _, scale := range opts {
		split := strings.Split(scale, "=")
		if len(split) != 2 {
			return fmt.Errorf("invalid --scale option %q. Should be SERVICE=NUM", scale)
		}
		name := split[0]
		replicas, err := strconv.Atoi(split[1])
		if err != nil {
			return err
		}
		err = setServiceScale(project, name, replicas)
		if err != nil {
			return err
		}
	}
	return nil
}

func setServiceScale(project *types.Project, name string, replicas int) error {
	for i, s := range project.Services {
		if s.Name == name {
			service, err := project.GetService(name)
			if err != nil {
				return err
			}
			if service.Deploy == nil {
				service.Deploy = &types.DeployConfig{}
			}
			count := uint64(replicas)
			service.Deploy.Replicas = &count
			project.Services[i] = service
			return nil
		}
	}
	return fmt.Errorf("unknown service %q", name)
}

func setup(ctx context.Context, opts composeOptions, services []string) (*client.Client, *types.Project, error) {
	c, err := client.NewWithDefaultLocalBackend(ctx)
	if err != nil {
		return nil, nil, err
	}

	project, err := opts.toProject(services)
	if err != nil {
		return nil, nil, err
	}

	if opts.DomainName != "" {
		// arbitrarily set the domain name on the first service ; ACI backend will expose the entire project
		project.Services[0].DomainName = opts.DomainName
	}
	if opts.Build {
		for i, service := range project.Services {
			service.PullPolicy = types.PullPolicyBuild
			project.Services[i] = service
		}
	}
	if opts.noBuild {
		for i, service := range project.Services {
			service.Build = nil
			project.Services[i] = service
		}
	}

	if opts.EnvFile != "" {
		var services types.Services
		for _, s := range project.Services {
			ef := opts.EnvFile
			if ef != "" {
				if !filepath.IsAbs(ef) {
					ef = filepath.Join(project.WorkingDir, opts.EnvFile)
				}
				if s.Labels == nil {
					s.Labels = make(map[string]string)
				}
				s.Labels[compose.EnvironmentFileLabel] = ef
				services = append(services, s)
			}
		}
		project.Services = services
	}

	return c, project, nil
}

type printer struct {
	queue chan compose.ContainerEvent
}

func (p printer) run(ctx context.Context, cascadeStop bool, exitCodeFrom string, consumer compose.LogConsumer, stopFn func() error) (int, error) { //nolint:unparam
	var aborting bool
	var count int
	for {
		event := <-p.queue
		switch event.Type {
		case compose.UserCancel:
			aborting = true
		case compose.ContainerEventAttach:
			consumer.Register(event.Name, event.Source)
			count++
		case compose.ContainerEventExit:
			if !aborting {
				consumer.Status(event.Name, event.Source, fmt.Sprintf("exited with code %d", event.ExitCode))
			}
			if cascadeStop {
				if !aborting {
					aborting = true
					fmt.Println("Aborting on container exit...")
					err := stopFn()
					if err != nil {
						return 0, err
					}
				}
				if exitCodeFrom == "" || exitCodeFrom == event.Service {
					logrus.Error(event.ExitCode)
					return event.ExitCode, nil
				}
			}
			count--
			if count == 0 {
				// Last container terminated, done
				return 0, nil
			}
		case compose.ContainerEventLog:
			if !aborting {
				consumer.Log(event.Name, event.Service, event.Source, event.Line)
			}
		}
	}
}
