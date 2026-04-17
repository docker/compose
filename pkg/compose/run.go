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

// runTarget is the internal representation of a run target (service or job).
// It carries only what's needed for one-off container execution: a name and
// the container specification. ServiceConfig-only fields (Deploy, Scale, Profiles)
// are intentionally excluded.
type runTarget struct {
	Name string
	types.ContainerSpec
}

type prepareRunResult struct {
	containerID string
	target      runTarget
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

	// If the target has post_start hooks, set up a goroutine that waits for
	// the container to start and then executes them. This is needed because
	// cmd.RunStart both starts and attaches to the container in one call,
	// so we can't run hooks sequentially between start and attach.
	var hookErrCh chan error
	if len(result.target.PostStart) > 0 {
		hookErrCh = make(chan error, 1)
		go func() {
			hookErrCh <- s.runPostStartHooksOnEvent(ctx, result.containerID, result.target, result.created)
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
func (s *composeService) runPostStartHooksOnEvent(ctx context.Context, containerID string, target runTarget, ctr container.Summary) error {
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

	for _, hook := range target.PostStart {
		if err := s.runHook(ctx, ctr, target.Name, target.Tty, hook, nil); err != nil {
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

	target, err := resolveRunTarget(project, opts)
	if err != nil {
		return prepareRunResult{}, err
	}

	applyRunOptions(project, &target, opts)

	if err := s.stdin().CheckTty(opts.Interactive, target.Tty); err != nil {
		return prepareRunResult{}, err
	}

	slug := stringid.GenerateRandomID()
	if target.ContainerName == "" {
		target.ContainerName = fmt.Sprintf("%[1]s%[4]s%[2]s%[4]srun%[4]s%[3]s", project.Name, target.Name, stringid.TruncateID(slug), api.Separator)
	}
	target.Restart = ""
	target.CustomLabels = target.CustomLabels.
		Add(api.SlugLabel, slug).
		Add(api.OneoffLabel, "True")

	// Only ensure image exists for the target, dependencies were already handled by startDependencies
	buildOpts := prepareBuildOptions(opts)
	if err := s.ensureImagesExists(ctx, project, buildOpts, opts.QuietPull); err != nil {
		return prepareRunResult{}, err
	}

	observedState, err := s.getContainers(ctx, project.Name, oneOffInclude, true)
	if err != nil {
		return prepareRunResult{}, err
	}

	if !opts.NoDeps {
		if err := s.waitDependencies(ctx, project, target.Name, target.DependsOn, observedState, 0); err != nil {
			return prepareRunResult{}, err
		}
	}

	err = newConvergence(project.ServiceNames(), observedState, nil, nil, s).resolveContainerReferences(&target.ContainerSpec)
	if err != nil {
		return prepareRunResult{}, err
	}

	err = s.ensureModels(ctx, project, opts.QuietPull)
	if err != nil {
		return prepareRunResult{}, err
	}

	createOpts := createOptions{
		AutoRemove:        opts.AutoRemove,
		AttachStdin:       opts.Interactive,
		UseNetworkAliases: opts.UseNetworkAliases,
		Labels:            mergeLabels(target.Labels, target.CustomLabels),
	}

	eventName := "Container " + target.ContainerName
	s.events.On(creatingEvent(eventName))
	created, err := s.createMobyContainer(ctx, project, target.Name, &target.ContainerSpec, nil, target.ContainerName, -1, nil, createOpts)
	if err != nil {
		if ctx.Err() == nil {
			s.events.On(api.Resource{
				ID:     eventName,
				Status: api.Error,
				Text:   err.Error(),
			})
		}
		return prepareRunResult{}, err
	}
	s.events.On(createdEvent(eventName))

	inspect, err := s.apiClient().ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil {
		return prepareRunResult{}, err
	}

	err = s.injectSecrets(ctx, project, target.Name, &target.ContainerSpec, inspect.Container.ID)
	if err != nil {
		return prepareRunResult{containerID: created.ID}, err
	}

	err = s.injectConfigs(ctx, project, target.Name, &target.ContainerSpec, inspect.Container.ID)
	return prepareRunResult{
		containerID: created.ID,
		target:      target,
		created:     created,
	}, err
}

// resolveRunTarget looks up the run target (service or job) in the project
// and returns a runTarget carrying only Name + ContainerSpec.
func resolveRunTarget(project *types.Project, opts api.RunOptions) (runTarget, error) {
	if opts.Job != "" {
		job, ok := project.Jobs[opts.Job]
		if !ok {
			return runTarget{}, fmt.Errorf("no such job: %s", opts.Job)
		}
		return runTarget{Name: job.Name, ContainerSpec: job.ContainerSpec}, nil
	}
	service, err := project.GetService(opts.Service)
	if err != nil {
		return runTarget{}, err
	}
	return runTarget{Name: service.Name, ContainerSpec: service.ContainerSpec}, nil
}

func prepareBuildOptions(opts api.RunOptions) *api.BuildOptions {
	if opts.Build == nil {
		return nil
	}
	// Create a copy of build options and restrict to only the target service/job
	buildOptsCopy := *opts.Build
	buildOptsCopy.Services = []string{opts.TargetName()}
	return &buildOptsCopy
}

func applyRunOptions(project *types.Project, target *runTarget, opts api.RunOptions) {
	target.Tty = opts.Tty
	target.StdinOpen = opts.Interactive
	target.ContainerName = opts.Name

	if len(opts.Command) > 0 {
		target.Command = opts.Command
	}
	if opts.User != "" {
		target.User = opts.User
	}

	if len(opts.CapAdd) > 0 {
		target.CapAdd = append(target.CapAdd, opts.CapAdd...)
		target.CapDrop = slices.DeleteFunc(target.CapDrop, func(e string) bool { return slices.Contains(opts.CapAdd, e) })
	}
	if len(opts.CapDrop) > 0 {
		target.CapDrop = append(target.CapDrop, opts.CapDrop...)
		target.CapAdd = slices.DeleteFunc(target.CapAdd, func(e string) bool { return slices.Contains(opts.CapDrop, e) })
	}
	if opts.WorkingDir != "" {
		target.WorkingDir = opts.WorkingDir
	}
	if opts.Entrypoint != nil {
		target.Entrypoint = opts.Entrypoint
		if len(opts.Command) == 0 {
			target.Command = []string{}
		}
	}
	if len(opts.Environment) > 0 {
		cmdEnv := types.NewMappingWithEquals(opts.Environment)
		overrideEnv := cmdEnv.Resolve(func(s string) (string, bool) {
			v, ok := envResolver(project.Environment)(s)
			return v, ok
		}).RemoveEmpty()
		if target.Environment == nil {
			target.Environment = types.MappingWithEquals{}
		}
		target.Environment.OverrideBy(overrideEnv)
	}
	for k, v := range opts.Labels {
		target.Labels = target.Labels.Add(k, v)
	}
}

func (s *composeService) startDependencies(ctx context.Context, project *types.Project, options api.RunOptions) error {
	project = project.WithServicesDisabled(options.TargetName())

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
