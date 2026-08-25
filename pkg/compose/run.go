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

func (s *composeService) RunOneOffContainer(ctx context.Context, project *types.Project, options api.RunOptions) (int, error) {
	result, err := s.prepareRun(ctx, project, options)
	if err != nil {
		return 0, err
	}

	// remove cancellable context signal handler so we can forward signals to container without compose exiting
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
		OpenStdin:  !options.Detach && options.Interactive,
		Attach:     !options.Detach,
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

func (s *composeService) prepareRun(ctx context.Context, project *types.Project, options api.RunOptions) (prepareRunResult, error) {
	// Temporary implementation of use_api_socket until we get actual support inside docker engine
	project, err := s.useAPISocket(project)
	if err != nil {
		return prepareRunResult{}, err
	}

	err = Run(ctx, func(ctx context.Context) error {
		return s.startDependencies(ctx, project, options)
	}, "run", s.events)
	if err != nil {
		return prepareRunResult{}, err
	}

	service, err := project.GetService(options.Service)
	if err != nil {
		return prepareRunResult{}, err
	}

	applyRunOptions(project, &service, options)

	if err := s.stdin().CheckTty(options.Interactive, service.Tty); err != nil {
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
	buildOpts := prepareBuildOptions(options)
	if err := s.ensureImagesExists(ctx, project, buildOpts, options.QuietPull); err != nil { // all dependencies already checked, but might miss service img
		return prepareRunResult{}, err
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return prepareRunResult{}, err
	}

	if !options.NoDeps {
		if err := s.waitDependencies(ctx, project, service.Name, service.DependsOn, observedState, 0); err != nil {
			return prepareRunResult{}, err
		}
	}
	createOpts := createOptions{
		AutoRemove:        options.AutoRemove,
		AttachStdin:       options.Interactive,
		UseNetworkAliases: options.UseNetworkAliases,
		Labels:            mergeLabels(service.Labels, service.CustomLabels),
	}

	if err := s.resolveRunServiceReferences(ctx, project.Name, &service); err != nil {
		return prepareRunResult{}, err
	}

	err = s.ensureModels(ctx, project, options.QuietPull)
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

func prepareBuildOptions(options api.RunOptions) *api.BuildOptions {
	if options.Build == nil {
		return nil
	}
	// Create a copy of build options and restrict to only the target service
	buildOptionsCopy := *options.Build
	buildOptionsCopy.Services = []string{options.Service}
	return &buildOptionsCopy
}

func applyRunOptions(project *types.Project, service *types.ServiceConfig, options api.RunOptions) {
	service.Tty = options.Tty
	service.StdinOpen = options.Interactive
	service.ContainerName = options.Name

	if len(options.Command) > 0 {
		service.Command = options.Command
	}
	if options.User != "" {
		service.User = options.User
	}

	if len(options.CapAdd) > 0 {
		service.CapAdd = append(service.CapAdd, options.CapAdd...)
		service.CapDrop = slices.DeleteFunc(service.CapDrop, func(e string) bool { return slices.Contains(options.CapAdd, e) })
	}
	if len(options.CapDrop) > 0 {
		service.CapDrop = append(service.CapDrop, options.CapDrop...)
		service.CapAdd = slices.DeleteFunc(service.CapAdd, func(e string) bool { return slices.Contains(options.CapDrop, e) })
	}
	if options.WorkingDir != "" {
		service.WorkingDir = options.WorkingDir
	}
	if options.Entrypoint != nil {
		service.Entrypoint = options.Entrypoint
		if len(options.Command) == 0 {
			service.Command = []string{}
		}
	}
	if len(options.Environment) > 0 {
		cmdEnv := types.NewMappingWithEquals(options.Environment)
		serviceOverrideEnv := cmdEnv.Resolve(func(s string) (string, bool) {
			v, ok := envResolver(project.Environment)(s)
			return v, ok
		}).RemoveEmpty()
		if service.Environment == nil {
			service.Environment = types.MappingWithEquals{}
		}
		service.Environment.OverrideBy(serviceOverrideEnv)
	}
	for k, v := range options.Labels {
		service.Labels = service.Labels.Add(k, v)
	}
}

func (s *composeService) resolveRunServiceReferences(ctx context.Context, projectName string, service *types.ServiceConfig) error {
	containersByService, err := s.getContainersByService(ctx, projectName)
	if err != nil {
		return err
	}
	return resolveServiceReferences(service, containersByService)
}

func (s *composeService) startDependencies(ctx context.Context, project *types.Project, options api.RunOptions) error {
	project = project.WithServicesDisabled(options.Service)

	// calls the unexported create/start, not the public Create/Start: this
	// already runs inside the "run" operation's Start/Done bracket (see
	// prepareRun), and the public variants would open a second, nested one
	// on the same shared bus.
	err := s.create(ctx, project, api.CreateOptions{
		Build:         options.Build,
		IgnoreOrphans: options.IgnoreOrphans,
		RemoveOrphans: options.RemoveOrphans,
		QuietPull:     options.QuietPull,
	})
	if err != nil {
		return err
	}

	if len(project.Services) > 0 {
		return s.start(ctx, project.Name, api.StartOptions{
			Project: project,
		}, nil)
	}
	return nil
}
