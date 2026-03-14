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
	"os/signal"
	"slices"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	cmd "github.com/docker/cli/cli/command/container"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/stringid"

	"github.com/docker/compose/v5/pkg/api"
)

type prepareRunResult struct {
	containerID string
	service     types.ServiceConfig
	created     container.Summary
}

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, opts api.RunOptions) (int, error) {
	result, err := s.prepareRun(ctx, project, opts)
	if err != nil {
		return 0, err
	}

	// remove cancellable context signal handler so we can forward signals to container without compose from exiting
	signal.Reset()

	sigc := make(chan os.Signal, 128)
	signal.Notify(sigc)
	go cmd.ForwardAllSignals(ctx, s.apiClient(), result.containerID, sigc)
	defer signal.Stop(sigc)

	// If the service has post_start hooks, set up a goroutine that waits for
	// the container to start and then executes them. This is needed because
	// cmd.RunStart both starts and attaches to the container in one call,
	// so we can't run hooks sequentially between start and attach.
	var hookErrCh chan error
	if len(result.service.PostStart) > 0 {
		hookErrCh = make(chan error, 1)
		go func() {
			hookErrCh <- s.runPostStartHooksOnEvent(ctx, result.containerID, result.service, result.created)
		}()
	}

	err = cmd.RunStart(ctx, s.dockerCli, &cmd.StartOptions{
		OpenStdin:  !opts.Detach && opts.Interactive,
		Attach:     !opts.Detach,
		Containers: []string{result.containerID},
		DetachKeys: s.configFile().DetachKeys,
	})

	// Wait for hooks to complete if they were started
	if hookErrCh != nil {
		if hookErr := <-hookErrCh; hookErr != nil && err == nil {
			err = hookErr
		}
	}

	var stErr cli.StatusError
	if errors.As(err, &stErr) {
		return stErr.StatusCode, nil
	}
	return 0, err
}

// runPostStartHooksOnEvent listens for the container's start event and executes
// post_start lifecycle hooks once the container is running.
func (s *composeService) runPostStartHooksOnEvent(ctx context.Context, containerID string, service types.ServiceConfig, ctr container.Summary) error {
	evtCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	res := s.apiClient().Events(evtCtx, client.EventsListOptions{
		Filters: make(client.Filters).
			Add("type", "container").
			Add("container", containerID).
			Add("event", string(events.ActionStart)),
	})

	// Wait for the container start event
	select {
	case <-evtCtx.Done():
		return evtCtx.Err()
	case err := <-res.Err:
		return err
	case <-res.Messages:
		// Container started, run hooks
	}

	for _, hook := range service.PostStart {
		if err := s.runHook(ctx, ctr, service, hook, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *composeService) prepareRun(ctx context.Context, project *types.Project, opts api.RunOptions) (prepareRunResult, error) {
	// Temporary implementation of use_api_socket until we get actual support inside docker engine
	project, err := s.useAPISocket(project)
	if err != nil {
		return prepareRunResult{}, err
	}

	err = Run(ctx, func(ctx context.Context) error {
		return s.startDependencies(ctx, project, opts)
	}, "run", s.events)
	if err != nil {
		return prepareRunResult{}, err
	}

	service, err := project.GetService(opts.Service)
	if err != nil {
		return prepareRunResult{}, err
	}

	applyRunOptions(project, &service, opts)

	if err := s.stdin().CheckTty(opts.Interactive, service.Tty); err != nil {
		return prepareRunResult{}, err
	}

	slug := stringid.GenerateRandomID()
	if service.ContainerName == "" {
		service.ContainerName = fmt.Sprintf("%[1]s%[4]s%[2]s%[4]srun%[4]s%[3]s", project.Name, service.Name, stringid.TruncateID(slug), api.Separator)
	}
	one := 1
	service.Scale = &one
	service.Restart = ""
	if service.Deploy != nil {
		service.Deploy.RestartPolicy = nil
	}
	service.CustomLabels = service.CustomLabels.
		Add(api.SlugLabel, slug).
		Add(api.OneoffLabel, "True")

	// Only ensure image exists for the target service, dependencies were already handled by startDependencies
	buildOpts := prepareBuildOptions(opts)
	if err := s.ensureImagesExists(ctx, project, buildOpts, opts.QuietPull); err != nil { // all dependencies already checked, but might miss service img
		return prepareRunResult{}, err
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return prepareRunResult{}, err
	}

	if !opts.NoDeps {
		if err := s.waitDependencies(ctx, project, service.Name, service.DependsOn, observedState, 0); err != nil {
			return prepareRunResult{}, err
		}
	}
	createOpts := createOptions{
		AutoRemove:        opts.AutoRemove,
		AttachStdin:       opts.Interactive,
		UseNetworkAliases: opts.UseNetworkAliases,
		Labels:            mergeLabels(service.Labels, service.CustomLabels),
	}

	err = newConvergence(project.ServiceNames(), observedState, nil, nil, s).resolveServiceReferences(&service)
	if err != nil {
		return prepareRunResult{}, err
	}

	err = s.ensureModels(ctx, project, opts.QuietPull)
	if err != nil {
		return prepareRunResult{}, err
	}

	created, err := s.createContainer(ctx, project, service, service.ContainerName, -1, createOpts)
	if err != nil {
		return prepareRunResult{}, err
	}

	inspect, err := s.apiClient().ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil {
		return prepareRunResult{}, err
	}

	err = s.injectSecrets(ctx, project, service, inspect.Container.ID)
	if err != nil {
		return prepareRunResult{containerID: created.ID}, err
	}

	err = s.injectConfigs(ctx, project, service, inspect.Container.ID)
	return prepareRunResult{
		containerID: created.ID,
		service:     service,
		created:     created,
	}, err
}

func prepareBuildOptions(opts api.RunOptions) *api.BuildOptions {
	if opts.Build == nil {
		return nil
	}
	// Create a copy of build options and restrict to only the target service
	buildOptsCopy := *opts.Build
	buildOptsCopy.Services = []string{opts.Service}
	return &buildOptsCopy
}

func applyRunOptions(project *types.Project, service *types.ServiceConfig, opts api.RunOptions) {
	service.Tty = opts.Tty
	service.StdinOpen = opts.Interactive
	service.ContainerName = opts.Name

	if len(opts.Command) > 0 {
		service.Command = opts.Command
	}
	if opts.User != "" {
		service.User = opts.User
	}

	if len(opts.CapAdd) > 0 {
		service.CapAdd = append(service.CapAdd, opts.CapAdd...)
		service.CapDrop = slices.DeleteFunc(service.CapDrop, func(e string) bool { return slices.Contains(opts.CapAdd, e) })
	}
	if len(opts.CapDrop) > 0 {
		service.CapDrop = append(service.CapDrop, opts.CapDrop...)
		service.CapAdd = slices.DeleteFunc(service.CapAdd, func(e string) bool { return slices.Contains(opts.CapDrop, e) })
	}
	if opts.WorkingDir != "" {
		service.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != nil {
		service.Entrypoint = opts.Entrypoint
		if len(opts.Command) == 0 {
			service.Command = []string{}
		}
	}
	if len(opts.Environment) > 0 {
		cmdEnv := types.NewMappingWithEquals(opts.Environment)
		serviceOverrideEnv := cmdEnv.Resolve(func(s string) (string, bool) {
			v, ok := envResolver(project.Environment)(s)
			return v, ok
		}).RemoveEmpty()
		if service.Environment == nil {
			service.Environment = types.MappingWithEquals{}
		}
		service.Environment.OverrideBy(serviceOverrideEnv)
	}
	for k, v := range opts.Labels {
		service.Labels = service.Labels.Add(k, v)
	}
}

func (s *composeService) startDependencies(ctx context.Context, project *types.Project, options api.RunOptions) error {
	project = project.WithServicesDisabled(options.Service)

	err := s.Create(ctx, project, api.CreateOptions{
		Build:         options.Build,
		IgnoreOrphans: options.IgnoreOrphans,
		RemoveOrphans: options.RemoveOrphans,
		QuietPull:     options.QuietPull,
	})
	if err != nil {
		return err
	}

	if len(project.Services) > 0 {
		return s.Start(ctx, project.Name, api.StartOptions{
			Project: project,
		})
	}
	return nil
}
