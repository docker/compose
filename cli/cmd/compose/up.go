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

	"github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/docker/compose-cli/api/compose"
	"github.com/docker/compose-cli/api/context/store"
	"github.com/docker/compose-cli/api/progress"
	"github.com/docker/compose-cli/cli/formatter"
	"github.com/docker/compose-cli/utils"
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
	Detach             bool
	Environment        []string
	removeOrphans      bool
	forceRecreate      bool
	noRecreate         bool
	recreateDeps       bool
	noStart            bool
	noDeps             bool
	cascadeStop        bool
	exitCodeFrom       string
	scale              []string
	noColor            bool
	noPrefix           bool
	timeChanged        bool
	timeout            int
	noInherit          bool
	attachDependencies bool
	quietPull          bool
}

func (opts upOptions) recreateStrategy() string {
	if opts.noRecreate {
		return compose.RecreateNever
	}
	if opts.forceRecreate {
		return compose.RecreateForce
	}
	return compose.RecreateDiverged
}

func (opts upOptions) dependenciesRecreateStrategy() string {
	if opts.noRecreate {
		return compose.RecreateNever
	}
	if opts.recreateDeps {
		return compose.RecreateForce
	}
	return compose.RecreateDiverged
}

func (opts upOptions) GetTimeout() *time.Duration {
	if opts.timeChanged {
		t := time.Duration(opts.timeout) * time.Second
		return &t
	}
	return nil
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
		err = setServiceScale(project, name, replicas)
		if err != nil {
			return err
		}
	}

	return nil
}

func upCommand(p *projectOptions, contextType string, backend compose.Service) *cobra.Command {
	opts := upOptions{
		composeOptions: &composeOptions{
			projectOptions: p,
		},
	}
	upCmd := &cobra.Command{
		Use:   "up [SERVICE...]",
		Short: "Create and start containers",
		PreRun: func(cmd *cobra.Command, args []string) {
			opts.timeChanged = cmd.Flags().Changed("timeout")
		},
		RunE: Adapt(func(ctx context.Context, args []string) error {
			switch contextType {
			case store.LocalContextType, store.DefaultContextType, store.EcsLocalSimulationContextType:
				if opts.exitCodeFrom != "" {
					opts.cascadeStop = true
				}
				if opts.Build && opts.noBuild {
					return fmt.Errorf("--build and --no-build are incompatible")
				}
				if opts.Detach && (opts.attachDependencies || opts.cascadeStop) {
					return fmt.Errorf("--detach cannot be combined with --abort-on-container-exit or --attach-dependencies")
				}
				if opts.forceRecreate && opts.noRecreate {
					return fmt.Errorf("--force-recreate and --no-recreate are incompatible")
				}
				if opts.recreateDeps && opts.noRecreate {
					return fmt.Errorf("--always-recreate-deps and --no-recreate are incompatible")
				}
				return runCreateStart(ctx, backend, opts, args)
			default:
				return runUp(ctx, backend, opts, args)
			}
		}),
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
		flags.BoolVar(&opts.recreateDeps, "always-recreate-deps", false, "Recreate dependent containers. Incompatible with --no-recreate.")
		flags.BoolVarP(&opts.noInherit, "renew-anon-volumes", "V", false, "Recreate anonymous volumes instead of retrieving data from the previous containers.")
		flags.BoolVar(&opts.attachDependencies, "attach-dependencies", false, "Attach to dependent containers.")
		flags.BoolVar(&opts.quietPull, "quiet-pull", false, "Pull without printing progress information.")
	}

	return upCmd
}

func runUp(ctx context.Context, backend compose.Service, opts upOptions, services []string) error {
	project, err := setup(*opts.composeOptions, services)
	if err != nil {
		return err
	}

	err = opts.apply(project, services)
	if err != nil {
		return err
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		return "", backend.Up(ctx, project, compose.UpOptions{
			Detach:    opts.Detach,
			QuietPull: opts.quietPull,
		})
	})
	return err
}

func runCreateStart(ctx context.Context, backend compose.Service, opts upOptions, services []string) error {
	project, err := setup(*opts.composeOptions, services)
	if err != nil {
		return err
	}

	err = opts.apply(project, services)
	if err != nil {
		return err
	}

	if len(project.Services) == 0 {
		return fmt.Errorf("no service selected")
	}

	_, err = progress.Run(ctx, func(ctx context.Context) (string, error) {
		err := backend.Create(ctx, project, compose.CreateOptions{
			Services:             services,
			RemoveOrphans:        opts.removeOrphans,
			Recreate:             opts.recreateStrategy(),
			RecreateDependencies: opts.dependenciesRecreateStrategy(),
			Inherit:              !opts.noInherit,
			Timeout:              opts.GetTimeout(),
			QuietPull:            opts.quietPull,
		})
		if err != nil {
			return "", err
		}
		if opts.Detach {
			err = backend.Start(ctx, project, compose.StartOptions{})
		}
		return "", err
	})
	if err != nil {
		return err
	}

	if opts.noStart {
		return nil
	}

	if opts.attachDependencies {
		services = nil
	}

	if opts.Detach {
		return nil
	}

	queue := make(chan compose.ContainerEvent)
	printer := printer{
		queue: queue,
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	stopFunc := func() error {
		ctx := context.Background()
		_, err := progress.Run(ctx, func(ctx context.Context) (string, error) {
			go func() {
				<-signalChan
				backend.Kill(ctx, project, compose.KillOptions{}) // nolint:errcheck
			}()

			return "", backend.Stop(ctx, project, compose.StopOptions{})
		})
		return err
	}
	go func() {
		<-signalChan
		queue <- compose.ContainerEvent{
			Type: compose.UserCancel,
		}
		fmt.Println("Gracefully stopping... (press Ctrl+C again to force)")
		stopFunc() // nolint:errcheck
	}()

	consumer := formatter.NewLogConsumer(ctx, os.Stdout, !opts.noColor, !opts.noPrefix)

	var exitCode int
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		code, err := printer.run(opts.cascadeStop, opts.exitCodeFrom, consumer, stopFunc)
		exitCode = code
		return err
	})

	err = backend.Start(ctx, project, compose.StartOptions{
		Attach: func(event compose.ContainerEvent) {
			queue <- event
		},
		Services: services,
	})
	if err != nil {
		return err
	}

	err = eg.Wait()
	if exitCode != 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		return cli.StatusError{StatusCode: exitCode, Status: errMsg}
	}
	return err
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

func setup(opts composeOptions, services []string) (*types.Project, error) {
	project, err := opts.toProject(services)
	if err != nil {
		return nil, err
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

	return project, nil
}

type printer struct {
	queue chan compose.ContainerEvent
}

func (p printer) run(cascadeStop bool, exitCodeFrom string, consumer compose.LogConsumer, stopFn func() error) (int, error) {
	var aborting bool
	var count int
	for {
		event := <-p.queue
		switch event.Type {
		case compose.UserCancel:
			aborting = true
		case compose.ContainerEventAttach:
			consumer.Register(event.Container)
			count++
		case compose.ContainerEventExit:
			if !aborting {
				consumer.Status(event.Container, fmt.Sprintf("exited with code %d", event.ExitCode))
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
				consumer.Log(event.Container, event.Service, event.Line)
			}
		}
	}
}
